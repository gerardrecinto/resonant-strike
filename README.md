# Resonant Strike

[![Go Reference](https://pkg.go.dev/badge/github.com/gerardrecinto/resonant-strike.svg)](https://pkg.go.dev/github.com/gerardrecinto/resonant-strike)
[![License](https://img.shields.io/github/license/gerardrecinto/resonant-strike)](LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/gerardrecinto/resonant-strike)](go.mod)

Rhythm-tactical brawler prototype. No face buttons: guard stances are held on
the shoulder buttons and triggers like frets on a fretboard, and strikes are
thrown by flicking the right stick on the beat of the fight's soundtrack.
Rollback netcode and an adaptive AI opponent are built directly on
[Joltrin](https://github.com/SharedCode/joltrin), an embedded Go storage
engine, rather than a general-purpose database.

![Terminal recording of the exhibition match: chord-stance strikes resolving on the beat, then a live rollback and resimulation partway through](docs/assets/demo.gif)

`go run ./cmd/demo` runs this yourself, a scripted two-fighter exhibition
match that exercises the real chord/timing/move resolution and, partway
through, an actual network misprediction: it reverts the rollback ledger and
resimulates with the corrected input, and the corrected outcome (a fighter's
health reverting because a predicted jab turns out to have been a slip)
comes out of the real code path, not a canned print statement.

## Core mechanics

**Chord stances.** L1, L2, R1, and R2 combine into a `StanceChord`, a 4-bit
mask with 16 possible combinations. Each combination is a physical guard:
which lines are covered, which footwork angle is available, which strikes
are loaded. There is no discrete "block" button, the chord you are holding
*is* your guard.

**Beat execution.** The right stick is the strike trigger, not a directional
input. A flick is judged against the current beat phase of a high-BPM track:
perfect, good, or a miss. Timing quality, not stick direction, decides
whether a strike lands clean.

**Muay Thai resolution.** A perfectly timed flick while holding a stance
resolves into an authentic action: a slip, a teep, a low kick, a clinch entry,
depending on the chord held and what the opponent's state allows. Releasing
a chord in the window just before the beat is a feint: it cancels whatever
the held chord would have executed and opens the opponent to a counter
instead.

**Training camp.** A progressive-overload meta-game between fights. Squat and
deadlift one-rep maxes and cardio capacity carry into a match as a
`ConditioningSnapshot`. Higher kinetic strength unlocks heavier strikes and
raises the stamina penalty for a missed rhythm input, there is no free lunch
for a bigger lift number.

## Why Joltrin, specifically

Two different subsystems, two different reasons, covered in full in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md):

- **Rollback ledger.** `inmemory.BtreeInterface[uint64, FrameSnapshot]`, keyed
  by frame number. In-process, no transaction, no disk I/O: a rollback that
  itself blocks on I/O defeats the reason to roll back instead of just
  waiting for the network. This is the same package Joltrin's own
  client-side WASM demo uses for its zero-server ACID showcase, the
  reasoning transfers directly.
- **Adaptive AI.** `ai/vector`, an IVF-style index (k-means centroids, not a
  flat scan) over recorded rhythm and stance exchanges. Queried for the
  nearest historical situations to a live one, to predict what a specific
  opponent tends to do next instead of reacting off a fixed table. Covered
  in [docs/VECTOR_SEARCH.md](docs/VECTOR_SEARCH.md), including where this
  actually has to run (not inside the WASM client, `ai/vector` needs a real
  transactional backend the browser sandbox does not have).

## What exists right now

```
rollback/
  state.go        StanceChord, InputFrame, FighterState, ConditioningSnapshot, FrameSnapshot
  ledger.go       Ledger: Record, At, Latest, RevertTo, over an inmemory B-Tree
  ledger_test.go  retention window, revert, resimulate-and-overwrite, chord bitmask
combat/
  resolve.go      chord+judgment -> MoveID, damage scoring, miss stamina cost
  resolve_test.go timing windows, single-chord resolution, feint, damage scaling
cmd/demo/
  main.go         scripted two-fighter exhibition match, CLI-only, real rollback demo
docs/
  ARCHITECTURE.md   game loop, audio-sync manager, rollback manager, Joltrin engine placement
  VECTOR_SEARCH.md  embedding design and querying for the adaptive AI
  assets/           demo.cast / demo.gif, the recording above
```

This is a CLI-playable slice, not the full game: a real rollback ledger, a
real (if partial) combat resolution table, and a scripted match that
exercises both end to end, including an actual revert-and-resimulate. The
combat table currently covers the empty chord plus each single-button hold
(L1/L2/R1/R2), not all 16 chord combinations docs/ARCHITECTURE.md describes,
see the package doc comment in `combat/resolve.go`. The audio-sync manager,
rendering/WASM client, and the `ai/vector` adaptive-AI integration described
in the docs are designed but not implemented yet.

## Building

```bash
git clone https://github.com/gerardrecinto/resonant-strike.git
cd resonant-strike
go build ./...
go test ./...
go run ./cmd/demo
```

Requires `github.com/sharedcode/joltrin/v5` at v5.7.0 or later. Earlier
tagged releases of Joltrin were not `go get`-able by any external module,
root's go.mod had no version suffix on a v2+ tag, which v5.7.0 fixes.

## Determinism

Everything in `rollback.FrameSnapshot` is a fixed-size integer: no floats, no
pointers, no maps. This state is designed to be hashed for desync detection
and diffed frame to frame across two machines that may not share a CPU
architecture or Go build (hashing itself is a follow-up, not implemented
yet). Floats round differently across platforms and compilers, fixed-point
integers do not. See the package doc comment in `rollback/state.go`.
