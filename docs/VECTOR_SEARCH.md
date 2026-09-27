# Vector Search Implementation Concept

Goal: recognize a player's rhythm and stance tendencies well enough for the
AI to counter-pick, without hand-coding pattern rules per opponent
archetype. This uses Joltrin's `ai/vector` package, an IVF-style index
(k-means centroids over vector clusters, not a flat scan) with a generic
payload type.

## Where this actually runs

`ai/vector.Open[T]` takes a real `sop.Transaction`, backed by `incfs` or
`infs`. Neither has a WASM-compatible transport today: no filesystem, no
Redis client in a browser sandbox. This subsystem runs server-side or as a
native companion process, not inside the WASM game client. The client's job
is to record raw telemetry during a match and ship it up between rounds; it
never calls `ai/vector` directly.

One more real constraint worth stating plainly: `ai/vector` lives in
Joltrin's `ai` Go module, which has never been given its own version tag.
An external project can depend on the root `joltrin` module once it is
correctly tagged, but pulling in `ai/vector` today means pinning a specific
commit rather than a released version, until that module gets tagged
independently. The code below is accurate to the real API as of the
commit checked, treat it as a design reference, not a ready-to-import
package.

## What gets embedded

Not raw input samples, a fixed-width feature vector per resolved exchange,
computed once when the Combat Resolver judges a flick:

| Index | Feature | Why |
| --- | --- | --- |
| 0-3 | One-hot: which chord was held (up to 4 dims for the most common guards, rest bucketed) | Which stance the player reaches for |
| 4 | Beat offset, signed, normalized to one beat period | Early vs. late tendency, not just hit/miss |
| 5 | Stick flick magnitude, normalized | Commitment: a light flick vs. a full-deflection swing |
| 6 | Frames since last chord change | Stance-holding vs. rapid stance-cycling |
| 7 | 1 if this exchange was a feint (chord released before the beat), else 0 | Feint frequency is its own tendency, separate from stance choice |
| 8 | Fighter's current stamina, normalized | Whether the tendency holds under fatigue |
| 9 | Opponent's active move at the time (categorical, embedded as a small fixed sub-vector) | Tendencies are often reactive, not unconditional |

This is a hand-built feature vector, not a learned embedding from a neural
encoder. That is deliberate for a first version: the dimensions are
individually meaningful, so a bad AI read is debuggable ("it over-indexed on
early feints under low stamina") instead of being an opaque latent vector. A
learned encoder is a reasonable v2 once there is enough recorded match data
to train one against something.

## Indexing

```go
type ExchangePayload struct {
    Fighter     rollback.FighterID
    Frame       uint64
    Judgment    rollback.TimingJudgment
    ResolvedMove rollback.MoveID
}

store, err := vector.Open[ExchangePayload](ctx, trans, "player_rhythm_"+playerID, vector.Config{
    UsageMode: ai.DynamicWithVectorCountTracking, // continuous updates across a live match, not a one-shot bulk load
})

err = store.Upsert(ctx, ai.Item[ExchangePayload]{
    ID:     fmt.Sprintf("%s-%d", playerID, frame),
    Vector: featureVector, // the 10-dim vector above
    Payload: ExchangePayload{Fighter: fighterID, Frame: frame, Judgment: judgment, ResolvedMove: move},
})
```

One domain (`ai/vector`'s term for an index namespace) per player, not one
global index. Cross-player pattern mining is a different, batch-oriented
question; the in-match adaptive read only ever needs "what does this
specific opponent tend to do," which is exactly what per-player domain
isolation gives for free.

## Querying for a counter-pick

When the AI needs to decide how to guard against the player's next likely
action, build the same 10-dim feature shape from the current live situation
(current stamina, frames since the AI's own last chord change, the AI's
current active move as the "opponent's active move" dimension from the
player's perspective) and query:

```go
hits, err := store.Query(ctx, currentSituationVector, 5, func(p ExchangePayload) bool {
    return p.Judgment != rollback.JudgmentMiss // ignore whiffed exchanges, they are not a real tendency
})
```

The `k=5` nearest historical exchanges to the current situation are the
AI's evidence for "here is what this player has actually done in situations
like this one." Majority `ResolvedMove` among the hits, weighted by how
close each hit's `Score` is, becomes the AI's prediction; the AI biases its
guard toward countering that move instead of a fixed reaction table.

## Why centroid-based, not a flat scan

A flat scan is fine at the vector counts a single match produces. It stops
being fine once a domain represents a player's history across many
sessions, which is the actual target: the interesting tendencies are the
ones that hold up over dozens of matches, not just the current one. IVF
querying only compares against vectors in the nearest few centroids instead
of the whole domain, and `ai/vector` already exposes `AddCentroid`,
`SplitCentroid`, and `Optimize` for growing and rebalancing that structure
as a player's history accumulates, without needing a different storage
engine once a single match's data stops being enough.
