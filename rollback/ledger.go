package rollback

import (
	"fmt"

	"github.com/sharedcode/joltrin/v5/inmemory"
)

// Ledger is the rollback state ledger for one match. It keeps the last
// Window frames in memory, indexed by frame number, so the simulation can
// jump back to any of them in O(log n) instead of replaying from frame 0.
//
// This runs on inmemory.BtreeInterface: no transaction, no disk I/O, no
// network call sits between a rollback and the resimulation that follows
// it. A rollback that itself blocks on I/O defeats the reason to roll back.
// Durable persistence (match history, training camp progression) is a
// separate concern handled outside this package, see docs/ARCHITECTURE.md.
type Ledger struct {
	frames inmemory.BtreeInterface[uint64, FrameSnapshot]
	window uint64
	latest uint64
}

// NewLedger returns a ledger retaining the last window frames. Size window
// from the actual matchmaking latency budget: GGPO-style netcode typically
// needs 6-8 frames of rollback depth at 60fps to cover one round trip on a
// same-continent connection, more for cross-region play.
func NewLedger(window uint64) *Ledger {
	return &Ledger{
		frames: inmemory.NewBtree[uint64, FrameSnapshot](true),
		window: window,
	}
}

// Record stores a snapshot for its Frame, evicting anything that falls
// outside the retention window as a result. Calling Record again for a
// frame at or before the current latest is expected during resimulation
// after a rollback, it overwrites the prior (mispredicted) snapshot.
func (l *Ledger) Record(snap FrameSnapshot) {
	l.frames.Upsert(snap.Frame, snap)
	if snap.Frame >= l.latest {
		l.latest = snap.Frame
	}
	if l.latest+1 > l.window {
		l.evictBefore(l.latest + 1 - l.window)
	}
}

// At returns the snapshot for frame f, if it is still within the retention
// window.
func (l *Ledger) At(f uint64) (FrameSnapshot, bool) {
	if !l.frames.Find(f, true) {
		return FrameSnapshot{}, false
	}
	return l.frames.GetCurrentValue(), true
}

// Latest returns the highest frame number currently recorded.
func (l *Ledger) Latest() uint64 { return l.latest }

// RevertTo discards every recorded frame after f and returns the snapshot
// to resume simulation from. The caller resimulates forward from f+1 with
// corrected inputs, calling Record again for each frame it recomputes.
func (l *Ledger) RevertTo(f uint64) (FrameSnapshot, error) {
	snap, ok := l.At(f)
	if !ok {
		return FrameSnapshot{}, fmt.Errorf("rollback: frame %d is outside the retention window", f)
	}
	for cur := f + 1; cur <= l.latest; cur++ {
		l.frames.Remove(cur)
	}
	l.latest = f
	return snap, nil
}

// evictBefore removes every frame strictly older than f. Keys are collected
// before any Remove call: mutating the tree while an active Range iterator
// is walking it is not something this type promises to support safely.
func (l *Ledger) evictBefore(f uint64) {
	if f == 0 {
		return
	}
	var stale []uint64
	for k := range l.frames.Range(0, f-1) {
		stale = append(stale, k)
	}
	for _, k := range stale {
		l.frames.Remove(k)
	}
}
