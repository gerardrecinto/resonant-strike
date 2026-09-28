// Package combat resolves a held StanceChord and a beat TimingJudgment into
// a MoveID, and scores the outcome of that move. This is the "table-driven"
// logic rollback.state.go's MoveID comment refers to: rollback stays a pure
// data model, this package is the rules layer built on top of it.
//
// The table here covers the four single-button chord holds, one Muay Thai
// action each, plus the empty chord (a plain jab) and a release-triggered
// feint. The full 16-combination chord space docs/ARCHITECTURE.md describes
// (every multi-button combo mapping to its own guard) is a follow-up, not
// implemented here: a chord with more than one button held currently
// resolves via the fixed priority order in resolve(), it does not yet get
// its own distinct move.
package combat

import "github.com/gerardrecinto/resonant-strike/rollback"

// JudgeTiming classifies frame's distance from the nearest beat, both given
// in frames. framesPerBeat, perfectWindow, and goodWindow are all whole
// frame counts so this stays fixed-point, consistent with everything else
// this simulation hashes.
func JudgeTiming(frame uint64, framesPerBeat uint64, perfectWindow uint64, goodWindow uint64) rollback.TimingJudgment {
	phase := frame % framesPerBeat
	dist := phase
	if framesPerBeat-phase < dist {
		dist = framesPerBeat - phase
	}
	switch {
	case dist <= perfectWindow:
		return rollback.JudgmentPerfect
	case dist <= goodWindow:
		return rollback.JudgmentGood
	default:
		return rollback.JudgmentMiss
	}
}

// feintPriority lists chord bits in the fixed precedence order Resolve uses
// when more than one bit is held: the first match wins. This is the
// placeholder for the full 16-entry table described in the package doc.
var feintPriority = []struct {
	bit  rollback.StanceChord
	move rollback.MoveID
}{
	{rollback.ChordL1, rollback.MoveTeep},
	{rollback.ChordR1, rollback.MoveLowKick},
	{rollback.ChordL2, rollback.MoveSlip},
	{rollback.ChordR2, rollback.MoveClinch},
}

// Resolve maps a held chord and its beat judgment to a MoveID. released
// marks that the chord was let go this frame; a release on anything but a
// miss resolves to a feint instead of the chord's usual move, per
// rollback.InputFrame.ChordReleased's doc comment.
func Resolve(chord rollback.StanceChord, judgment rollback.TimingJudgment, released bool) rollback.MoveID {
	if judgment == rollback.JudgmentMiss || judgment == rollback.JudgmentNone {
		return rollback.MoveNone
	}
	if released {
		return rollback.MoveFeint
	}
	for _, entry := range feintPriority {
		if chord.Has(entry.bit) {
			return entry.move
		}
	}
	return rollback.MoveJab
}

// IsDefensive reports whether move is an evasive action rather than a
// strike, so an exchange resolver knows not to count it as an opening.
func IsDefensive(move rollback.MoveID) bool {
	return move == rollback.MoveSlip
}

// baseDamage is each move's damage on a JudgmentPerfect hit, in whole health
// points. A JudgmentGood hit scores 60% of this, integer division.
func baseDamage(move rollback.MoveID) int16 {
	switch move {
	case rollback.MoveJab:
		return 5
	case rollback.MoveTeep:
		return 8
	case rollback.MoveLowKick:
		return 7
	case rollback.MoveClinch:
		return 4
	default:
		return 0
	}
}

// Damage returns how much health a move deals when it lands with judgment.
// MoveSlip, MoveFeint, MoveNone, and a miss all deal zero, they are not
// strikes.
func Damage(move rollback.MoveID, judgment rollback.TimingJudgment) int16 {
	base := baseDamage(move)
	if base == 0 {
		return 0
	}
	if judgment == rollback.JudgmentGood {
		return base * 6 / 10
	}
	return base
}

// clinchStaminaDrain is the extra stamina cost MoveClinch imposes on the
// fighter being clinched, on top of any damage.
const clinchStaminaDrain = int16(5)

// StaminaDrainOnDefender returns the stamina cost move inflicts on the
// opponent it lands on, separate from health damage.
func StaminaDrainOnDefender(move rollback.MoveID) int16 {
	if move == rollback.MoveClinch {
		return clinchStaminaDrain
	}
	return 0
}

// missStaminaBaseCost is the stamina lost by the attacking fighter on a
// missed rhythm input, before ConditioningSnapshot.MissPenaltyScale scales
// it. See MissStaminaCost.
const missStaminaBaseCost = int16(2)

// MissStaminaCost scales missStaminaBaseCost by the fighter's own
// MissPenaltyScale (basis points, 10000 = 1.0x), per the field's doc
// comment in rollback.ConditioningSnapshot: heavier kinetic load costs more
// stamina off-beat.
func MissStaminaCost(cond rollback.ConditioningSnapshot) int16 {
	scale := int32(cond.MissPenaltyScale)
	if scale == 0 {
		scale = 10000
	}
	return int16(int32(missStaminaBaseCost) * scale / 10000)
}
