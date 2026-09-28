package rollback

import "testing"

func snapshotAt(frame uint64) FrameSnapshot {
	return FrameSnapshot{
		Frame: frame,
		Fighters: [2]FighterState{
			{Frame: frame, Fighter: FighterP1, Health: 100},
			{Frame: frame, Fighter: FighterP2, Health: 100},
		},
	}
}

func Test_Ledger_RecordAndAt(t *testing.T) {
	l := NewLedger(8)
	l.Record(snapshotAt(0))
	l.Record(snapshotAt(1))

	snap, ok := l.At(1)
	if !ok {
		t.Fatalf("expected frame 1 to be present")
	}
	if snap.Frame != 1 {
		t.Fatalf("got frame %d, want 1", snap.Frame)
	}
	if l.Latest() != 1 {
		t.Fatalf("Latest() = %d, want 1", l.Latest())
	}
}

func Test_Ledger_AtMissingFrame(t *testing.T) {
	l := NewLedger(8)
	l.Record(snapshotAt(0))

	if _, ok := l.At(5); ok {
		t.Fatalf("expected frame 5 to be absent")
	}
}

func Test_Ledger_EvictsOutsideWindow(t *testing.T) {
	l := NewLedger(4)
	for f := uint64(0); f <= 10; f++ {
		l.Record(snapshotAt(f))
	}

	// Window 4: frames 7,8,9,10 should remain, 0-6 evicted.
	for f := uint64(0); f <= 6; f++ {
		if _, ok := l.At(f); ok {
			t.Fatalf("frame %d should have been evicted", f)
		}
	}
	for f := uint64(7); f <= 10; f++ {
		if _, ok := l.At(f); !ok {
			t.Fatalf("frame %d should still be retained", f)
		}
	}
}

func Test_Ledger_EvictBeforeDoesNotUnderflowAtFrameZero(t *testing.T) {
	l := NewLedger(4)
	// Recording only frame 0 must not panic evicting "before" it.
	l.Record(snapshotAt(0))

	if _, ok := l.At(0); !ok {
		t.Fatalf("frame 0 should still be present")
	}
}

func Test_Ledger_RevertToDiscardsLaterFrames(t *testing.T) {
	l := NewLedger(16)
	for f := uint64(0); f <= 5; f++ {
		l.Record(snapshotAt(f))
	}

	snap, err := l.RevertTo(2)
	if err != nil {
		t.Fatalf("RevertTo(2): %v", err)
	}
	if snap.Frame != 2 {
		t.Fatalf("got frame %d, want 2", snap.Frame)
	}
	if l.Latest() != 2 {
		t.Fatalf("Latest() = %d, want 2 after revert", l.Latest())
	}
	for f := uint64(3); f <= 5; f++ {
		if _, ok := l.At(f); ok {
			t.Fatalf("frame %d should have been discarded by RevertTo", f)
		}
	}
}

func Test_Ledger_RevertToThenResimulateOverwrites(t *testing.T) {
	l := NewLedger(16)
	for f := uint64(0); f <= 5; f++ {
		l.Record(snapshotAt(f))
	}

	if _, err := l.RevertTo(2); err != nil {
		t.Fatalf("RevertTo(2): %v", err)
	}

	// Resimulate frame 3 with a corrected snapshot (different health, e.g.
	// the corrected remote input changed the outcome).
	corrected := snapshotAt(3)
	corrected.Fighters[1].Health = 80
	l.Record(corrected)

	got, ok := l.At(3)
	if !ok {
		t.Fatalf("frame 3 should be present after resimulation")
	}
	if got.Fighters[1].Health != 80 {
		t.Fatalf("Fighters[1].Health = %d, want 80 (corrected)", got.Fighters[1].Health)
	}
	if l.Latest() != 3 {
		t.Fatalf("Latest() = %d, want 3", l.Latest())
	}
}

func Test_Ledger_RevertToOutsideWindowErrors(t *testing.T) {
	l := NewLedger(4)
	for f := uint64(0); f <= 10; f++ {
		l.Record(snapshotAt(f))
	}

	if _, err := l.RevertTo(0); err == nil {
		t.Fatalf("expected RevertTo(0) to fail, frame 0 is outside the retention window")
	}
}

func Test_Ledger_RevertToLatestIsNoOp(t *testing.T) {
	l := NewLedger(8)
	for f := uint64(0); f <= 3; f++ {
		l.Record(snapshotAt(f))
	}

	snap, err := l.RevertTo(l.Latest())
	if err != nil {
		t.Fatalf("RevertTo(latest): %v", err)
	}
	if snap.Frame != 3 {
		t.Fatalf("got frame %d, want 3", snap.Frame)
	}
	if l.Latest() != 3 {
		t.Fatalf("Latest() = %d, want 3 unchanged", l.Latest())
	}
	for f := uint64(0); f <= 3; f++ {
		if _, ok := l.At(f); !ok {
			t.Fatalf("frame %d should still be present after a no-op revert", f)
		}
	}
}

func Test_Ledger_RevertToFutureFrameErrors(t *testing.T) {
	l := NewLedger(8)
	l.Record(snapshotAt(0))
	l.Record(snapshotAt(1))

	if _, err := l.RevertTo(5); err == nil {
		t.Fatalf("expected RevertTo(5) to fail, frame 5 has not been simulated yet")
	}
}

func Test_NewLedger_ZeroWindowPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected NewLedger(0) to panic")
		}
	}()
	NewLedger(0)
}

func Test_StanceChord_Has(t *testing.T) {
	c := ChordL1 | ChordR2
	if !c.Has(ChordL1) {
		t.Fatalf("expected chord to have L1")
	}
	if !c.Has(ChordR2) {
		t.Fatalf("expected chord to have R2")
	}
	if c.Has(ChordL2) {
		t.Fatalf("did not expect chord to have L2")
	}
	if c.Has(ChordL1 | ChordL2) {
		t.Fatalf("did not expect chord to satisfy L1+L2, only L1 is held")
	}
}
