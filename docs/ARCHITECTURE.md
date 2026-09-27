# Architecture

Resonant Strike is a two-player rhythm-tactical brawler. Combat resolves on
beat, netcode is rollback-based, and the AI adapts to a player's rhythm and
stance tendencies over the course of a match. This document covers the four
subsystems that talk to each other every frame and where each one actually
runs.

## Component breakdown

```
                    +------------------+
                    |   Audio Clock    |
                    |  (BPM, beat pos) |
                    +---------+--------+
                              |
                     beat phase, tick
                              |
                              v
  Local Input ---> +------------------+ <--- Remote Input
  (chord + stick)  |    Game Loop     |      (network, delayed)
                    |  (fixed-step,   |
                    |   60 Hz)        |
                    +---------+--------+
                              |
                    InputFrame per fighter
                              |
                              v
                    +------------------+
                    | Rollback Manager |
                    |  (rollback pkg)  |
                    +---------+--------+
                        |            |
              Record/At/RevertTo   simulate(frame, inputs) -> FighterState
                        |            |
                        v            v
              +------------------+  +------------------+
              | inmemory.Btree   |  |  Combat Resolver  |
              | (frame ledger)   |  |  (chord + beat ->  |
              |  keyed by frame  |  |   Muay Thai move)  |
              +------------------+  +------------------+
                                              |
                                    telemetry sample
                                    (chord, flick timing,
                                     judgment) per resolved
                                     input
                                              |
                                              v
                                  +------------------------+
                                  |  Adaptive AI pipeline   |
                                  |  (ai/vector, server or  |
                                  |   native process, not   |
                                  |   the WASM client)      |
                                  +------------------------+
```

## Game loop

Fixed-step simulation at 60 Hz, matching the beat-execution requirement:
timing judgment is computed against the audio clock's beat phase, and a fixed
step is what makes that judgment reproducible across a rollback resimulation.
A variable-step loop would compute a different beat phase on replay than it
did the first time, corrupting the exact thing rollback depends on being
deterministic.

Each tick:

1. Read local `InputFrame` (chord held, stick deflection, chord-release
   flag).
2. Receive or predict the remote `InputFrame` for the same frame number.
3. Hand both to the Rollback Manager.
4. Rollback Manager checks whether the remote input for any frame still
   within the retention window turned out to differ from what was predicted.
   If so: `RevertTo` that frame, then resimulate forward to the current
   frame with the corrected input.
5. Simulate the current frame: Combat Resolver reads each fighter's held
   `StanceChord`, the stick flick against the Audio Clock's current beat
   phase, and the prior `FighterState`, and produces the new
   `FighterState` for both fighters (judgment, resolved move, position,
   health, stamina).
6. `Ledger.Record` the resulting `FrameSnapshot`.
7. Render from the just-computed `FighterState`.

## Audio-sync manager

Owns the BPM table for the current track and exposes one query the Combat
Resolver calls every tick: given the current frame number, how far into the
current beat window is it, in the same fixed-point units `InputFrame` uses
for stick deflection. This has to be a pure function of frame number (frame
count times seconds-per-frame times BPM, not a wall-clock timestamp), for the
same determinism reason the game loop is fixed-step: resimulating frame 118
after a rollback must compute the exact same beat phase it computed the first
time frame 118 ran, or the judgment changes on replay and state diverges
between the two peers.

Audio playback itself (the actual samples hitting speakers) is not on this
hot path. It free-runs against the same BPM table and frame-to-time mapping,
correction on drift is a presentation-layer concern, not a simulation input.

## Rollback manager

`rollback.Ledger`, backed by `inmemory.BtreeInterface[uint64, FrameSnapshot]`.
See [Go Structs for Rollback](#go-structs-for-rollback) below for the actual
types. Two properties matter architecturally:

- **In-memory, not transactional.** `inmemory.NewBtree` has no disk I/O and
  no transaction to begin or commit, every operation is a direct B-Tree call.
  A rollback that itself has to wait on I/O defeats the point of rolling
  back instead of just waiting for the network. This is the same package
  Joltrin's own client-side WASM demo already uses for its zero-server ACID
  showcase (`demo/main.go` in the joltrin repo), same reasoning applies here.
- **Keyed by frame number, not a ring buffer index.** A B-Tree lookup by
  frame number is O(log n) regardless of how the frame arrived (in order,
  or as part of a resimulation run), and `Range` gives a cheap way to find
  and evict everything outside the retention window without tracking
  wraparound arithmetic by hand.

Durable state, the training-camp progression a player carries between
matches, is a different lifetime than in-match frames and is out of scope for
`rollback.Ledger`. It gets snapshotted at session boundaries, not per frame,
and can use Joltrin's transactional backends (`incfs`/`infs` on a native
build, or the OPFS bridge pattern the WASM technical demo already
establishes for browser clients) without needing sub-millisecond access.

## Wasm-compiled Joltrin engine

Two different deployment placements, not one:

- **Client (WASM).** Compiles against `github.com/sharedcode/joltrin/v5/inmemory`
  only. No transaction manager, no B-Tree persistence backend, no cgo. This
  is exactly what makes it embeddable in a browser sandbox with no external
  database driver, which was the whole point of running Joltrin client-side.
- **Adaptive AI (native or server process).** `ai/vector`'s `Open[T]` needs a
  real `sop.Transaction`, which means a real persistence backend (`incfs` or
  `infs`), which means a real filesystem or a cache client (Redis), neither
  of which exists inside a WASM sandbox. This piece cannot run on the client
  today without a browser-side transactional backend Joltrin does not
  currently have (the client's OPFS access is bridged manually at the
  application layer today, not through `incfs`/`infs`; see
  [Vector Search Implementation Concept](VECTOR_SEARCH.md) for what that
  gap concretely means for where this runs).

Practically: the client streams resolved combat telemetry (chord held,
flick timing, judgment, resulting move) up to wherever the vector store
actually lives, batched between rounds, not per frame, this is training
signal for pattern-matching a player's tendencies, not a rollback-critical
path with a 16ms budget.

## Go Structs for Rollback

Defined in `rollback/state.go` and `rollback/ledger.go`:

- `StanceChord`: bitmask over L1/L2/R1/R2.
- `InputFrame`: one player's raw input for one tick. The only thing that
  crosses the network.
- `FighterState`: one fighter's full derived state for one tick. Never
  mutated directly, always recomputed from `InputFrame` history.
- `ConditioningSnapshot`: training-camp stats captured at match start.
- `FrameSnapshot`: both fighters' state plus both inputs for one frame, the
  ledger's storage unit.
- `Ledger`: `Record`, `At`, `RevertTo`, `Latest`, backed by
  `inmemory.BtreeInterface[uint64, FrameSnapshot]`.

All fixed-point integers, no floats, no pointers, no maps in anything that
gets hashed or compared across peers. See the package doc comment in
`rollback/state.go` for why.
