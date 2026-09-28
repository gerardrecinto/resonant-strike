package main

import (
	"strings"
	"testing"

	"github.com/gerardrecinto/resonant-strike/rollback"
)

func Test_chordName_UnknownBitsDoNotPanic(t *testing.T) {
	// StanceChord is a bare uint8; a bit outside the four defined chord
	// buttons is invalid but must not crash the print path.
	got := chordName(rollback.StanceChord(1 << 5))
	if !strings.HasPrefix(got, "unknown(") {
		t.Fatalf("chordName(unknown bit) = %q, want an \"unknown(...)\" fallback", got)
	}
}

func Test_chordName_KnownChords(t *testing.T) {
	if got := chordName(0); got != "none" {
		t.Fatalf("chordName(0) = %q, want none", got)
	}
	if got := chordName(rollback.ChordL1); got != "L1" {
		t.Fatalf("chordName(L1) = %q, want L1", got)
	}
	if got := chordName(rollback.ChordL1 | rollback.ChordR2); got != "L1+R2" {
		t.Fatalf("chordName(L1+R2) = %q, want L1+R2", got)
	}
}

func Test_clampNonNegative(t *testing.T) {
	v := int16(-5)
	clampNonNegative(&v)
	if v != 0 {
		t.Fatalf("clampNonNegative(-5) = %d, want 0", v)
	}
	v = 42
	clampNonNegative(&v)
	if v != 42 {
		t.Fatalf("clampNonNegative(42) = %d, want unchanged 42", v)
	}
}
