# Resonant Strike

Rhythm-tactical brawler prototype. Chord-stance combat resolved on beat,
rollback netcode, adaptive AI over player rhythm and stance tendencies.

## Core mechanics

- **Input**: no face buttons. L1/L2/R1/R2 combine into a `StanceChord`, a
  guard held like a chord on a fretboard.
- **Execution**: right stick is the strike trigger. A flick lands on-beat
  against a high-BPM track, or the game judges it early, late, or a miss.
- **Combat**: a perfectly timed flick while holding a stance resolves into a
  Muay Thai action, slip, feint, footwork, strike, depending on the chord.
  Releasing a stance right before the beat is a feint, it cancels the guard
  to open a counter instead of executing it.
- **Conditioning**: a training-camp meta-game between fights. Squat, deadlift,
  and cardio capacity trade off: higher kinetic strength unlocks heavier
  strikes and a steeper stamina penalty for missed rhythm inputs.

## Why Joltrin

The rollback ledger is a `github.com/sharedcode/joltrin/v5/inmemory` B-Tree
keyed by frame number, sub-millisecond, in-process, no transaction and no
disk I/O between a rollback and the resimulation that follows it. The
adaptive AI is `ai/vector`, an IVF-style vector index over recorded rhythm
and stance exchanges, queried for the nearest historical situations to
predict what a specific opponent tends to do next.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the game loop,
audio-sync manager, rollback manager, and Joltrin engine talk to each other,
and [docs/VECTOR_SEARCH.md](docs/VECTOR_SEARCH.md) for the embedding design
behind the adaptive AI.

## Status

Prototype. `rollback/` has the frame-ledger data model and passes its test
suite. Combat resolution, the audio-sync manager, and the AI pipeline
described in the architecture doc are not implemented yet.

## Requirements

Depends on `github.com/sharedcode/joltrin/v5`. The root module's tagged
releases were not `go get`-able before that `/v5` path suffix landed
(Go requires a matching path suffix for any v2+ tag), pin to a release at or
after that fix.
