package combat

import (
	"testing"

	"github.com/gerardrecinto/resonant-strike/rollback"
)

func Test_JudgeTiming(t *testing.T) {
	const framesPerBeat = 30
	cases := []struct {
		frame uint64
		want  rollback.TimingJudgment
	}{
		{frame: 0, want: rollback.JudgmentPerfect},
		{frame: 30, want: rollback.JudgmentPerfect},
		{frame: 1, want: rollback.JudgmentPerfect},
		{frame: 29, want: rollback.JudgmentPerfect},
		{frame: 4, want: rollback.JudgmentGood},
		{frame: 26, want: rollback.JudgmentGood},
		{frame: 15, want: rollback.JudgmentMiss},
	}
	for _, c := range cases {
		got := JudgeTiming(c.frame, framesPerBeat, 2, 5)
		if got != c.want {
			t.Fatalf("JudgeTiming(%d) = %v, want %v", c.frame, got, c.want)
		}
	}
}

func Test_Resolve_EmptyChordIsJab(t *testing.T) {
	if got := Resolve(0, rollback.JudgmentPerfect, false); got != rollback.MoveJab {
		t.Fatalf("Resolve(empty chord) = %v, want MoveJab", got)
	}
}

func Test_Resolve_MissIsNoMove(t *testing.T) {
	if got := Resolve(rollback.ChordL1, rollback.JudgmentMiss, false); got != rollback.MoveNone {
		t.Fatalf("Resolve(miss) = %v, want MoveNone", got)
	}
}

func Test_Resolve_ReleaseIsFeint(t *testing.T) {
	if got := Resolve(rollback.ChordL2, rollback.JudgmentGood, true); got != rollback.MoveFeint {
		t.Fatalf("Resolve(released) = %v, want MoveFeint", got)
	}
}

func Test_Resolve_SingleChordMoves(t *testing.T) {
	cases := []struct {
		chord rollback.StanceChord
		want  rollback.MoveID
	}{
		{rollback.ChordL1, rollback.MoveTeep},
		{rollback.ChordR1, rollback.MoveLowKick},
		{rollback.ChordL2, rollback.MoveSlip},
		{rollback.ChordR2, rollback.MoveClinch},
	}
	for _, c := range cases {
		if got := Resolve(c.chord, rollback.JudgmentPerfect, false); got != c.want {
			t.Fatalf("Resolve(%v) = %v, want %v", c.chord, got, c.want)
		}
	}
}

func Test_Damage_GoodIsSixtyPercentOfPerfect(t *testing.T) {
	perfect := Damage(rollback.MoveTeep, rollback.JudgmentPerfect)
	good := Damage(rollback.MoveTeep, rollback.JudgmentGood)
	if perfect != 8 {
		t.Fatalf("perfect teep damage = %d, want 8", perfect)
	}
	if good != 4 {
		t.Fatalf("good teep damage = %d, want 4 (60%% of 8, integer division)", good)
	}
}

func Test_Damage_DefensiveMovesDealNone(t *testing.T) {
	if got := Damage(rollback.MoveSlip, rollback.JudgmentPerfect); got != 0 {
		t.Fatalf("slip damage = %d, want 0", got)
	}
	if got := Damage(rollback.MoveFeint, rollback.JudgmentPerfect); got != 0 {
		t.Fatalf("feint damage = %d, want 0", got)
	}
}

func Test_MissStaminaCost_ScalesWithConditioning(t *testing.T) {
	baseline := MissStaminaCost(rollback.ConditioningSnapshot{})
	if baseline != missStaminaBaseCost {
		t.Fatalf("baseline miss cost = %d, want %d", baseline, missStaminaBaseCost)
	}
	scaled := MissStaminaCost(rollback.ConditioningSnapshot{MissPenaltyScale: 15000})
	if scaled != 3 {
		t.Fatalf("1.5x miss cost = %d, want 3", scaled)
	}
}

func Test_IsDefensive(t *testing.T) {
	if !IsDefensive(rollback.MoveSlip) {
		t.Fatalf("expected MoveSlip to be defensive")
	}
	if IsDefensive(rollback.MoveJab) {
		t.Fatalf("did not expect MoveJab to be defensive")
	}
}

func Test_JudgeTiming_ZeroFramesPerBeatPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected JudgeTiming with framesPerBeat=0 to panic")
		}
	}()
	JudgeTiming(0, 0, 2, 5)
}
