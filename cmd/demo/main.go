// Command demo runs a short, fully scripted exhibition match on the terminal:
// two fighters exchanging chord-stance strikes on the beat, resolved through
// the combat package, recorded into a rollback.Ledger one action at a time,
// and then an actual network misprediction and rollback partway through, so
// the resimulation isn't just described, it runs and its corrected outcome
// prints next to the mispredicted one it replaced.
//
// The ledger is indexed by ledger sequence number, one per resolved action,
// not by raw 60fps frame count: a real simulation ticks (and records) every
// frame regardless of whether an input changed, this demo only resolves a
// frame when a scripted action happens on it, so it records only those. The
// beat/frame math used for on-beat timing judgment is unaffected by that
// choice, see beatFrame below.
//
// Nothing here is randomized: every input is scripted so the match plays out
// identically on every run, the same property rollback netcode depends on.
package main

import (
	"fmt"
	"time"

	"github.com/gerardrecinto/resonant-strike/combat"
	"github.com/gerardrecinto/resonant-strike/rollback"
)

const (
	framesPerBeat = 30 // 60fps at 120 BPM, used for the timing-judgment phase only
	perfectWindow = 2
	goodWindow    = 5
	ledgerWindow  = 8 // matches the 6-8 frame rollback depth ARCHITECTURE.md calls for
)

// beatEvent is one scripted action for one fighter on a given beat.
type beatEvent struct {
	fighter  rollback.FighterID
	beat     uint64
	chord    rollback.StanceChord
	released bool
}

func main() {
	fmt.Println(bold("RESONANT STRIKE") + " -- exhibition match, seed: scripted, no RNG")
	fmt.Println(dim("120 BPM, 30 frames/beat, rollback window 8 ledger entries"))
	fmt.Println()

	p1 := rollback.FighterState{
		Fighter: rollback.FighterP1, Health: 100, Stamina: 100,
		Conditioning: rollback.ConditioningSnapshot{SquatMaxLb: 405, DeadliftMaxLb: 495, CardioVO2Max: 520, MissPenaltyScale: 10000},
	}
	p2 := rollback.FighterState{
		Fighter: rollback.FighterP2, Health: 100, Stamina: 100,
		Conditioning: rollback.ConditioningSnapshot{SquatMaxLb: 315, DeadliftMaxLb: 385, CardioVO2Max: 610, MissPenaltyScale: 8500},
	}

	ledger := rollback.NewLedger(ledgerWindow)
	ledger.Record(rollback.FrameSnapshot{Frame: 0, Fighters: [2]rollback.FighterState{p1, p2}, Confirmed: true})

	// P1 presses the attack every odd beat, P2 answers every even beat. Beat
	// 6 (ledger sequence 6) is where the demo's rollback moment lives: it
	// gets resolved once here as a misprediction, then corrected below.
	script := []beatEvent{
		{rollback.FighterP1, 1, 0, false},                // jab, on beat
		{rollback.FighterP2, 2, rollback.ChordL2, false}, // slip, on beat
		{rollback.FighterP1, 3, rollback.ChordL1, false}, // teep, on beat
		{rollback.FighterP2, 4, rollback.ChordL2, false}, // slip, on beat
		{rollback.FighterP1, 5, rollback.ChordR1, false}, // low kick, on beat
		{rollback.FighterP2, 6, 0, false},                // MISPREDICTED locally as "no guard"
		{rollback.FighterP1, 7, rollback.ChordR2, false}, // clinch, on beat
		{rollback.FighterP2, 8, rollback.ChordL1, true},  // feint (release)
		{rollback.FighterP1, 9, rollback.ChordL1, false}, // teep, on beat
	}

	const mispredictSeq = 6
	const mispredictedP2Chord = rollback.StanceChord(0) // what we guessed while waiting on the network
	const correctedP2Chord = rollback.ChordL2           // what P2's input actually was, arrives late

	for i, ev := range script {
		seq := uint64(i + 1)
		p1, p2 = simulateBeat(ledger, seq, ev, p1, p2)

		if seq == mispredictSeq {
			runRollbackDemo(ledger, seq, &p1, &p2, mispredictedP2Chord, correctedP2Chord)
		}
	}

	fmt.Println()
	fmt.Println(bold("-- final state --"))
	printFighter(p1)
	printFighter(p2)
	fmt.Printf("ledger: latest sequence %d, retention window %d entries\n",
		ledger.Latest(), ledgerWindow)
}

// simulateBeat resolves one fighter's action, applies it to both fighters'
// state, records the resulting snapshot at ledger sequence seq, and prints
// what happened. It returns the updated pair so the caller threads state
// forward.
func simulateBeat(ledger *rollback.Ledger, seq uint64, ev beatEvent, p1, p2 rollback.FighterState) (rollback.FighterState, rollback.FighterState) {
	beatFrame := ev.beat * framesPerBeat
	judgment := combat.JudgeTiming(beatFrame, framesPerBeat, perfectWindow, goodWindow)
	move := combat.Resolve(ev.chord, judgment, ev.released)

	attacker, defender := &p1, &p2
	if ev.fighter == rollback.FighterP2 {
		attacker, defender = &p2, &p1
	}
	applyMove(attacker, defender, seq, ev.chord, judgment, move)

	ledger.Record(rollback.FrameSnapshot{
		Frame:     seq,
		Fighters:  [2]rollback.FighterState{p1, p2},
		Inputs:    inputsFor(beatFrame, ev),
		Confirmed: true,
	})

	printBeat(ev.beat, ev.fighter, ev.chord, judgment, move, combat.Damage(move, judgment), ev.released)
	return p1, p2
}

// applyMove mutates attacker and defender in place for one resolved move.
func applyMove(attacker, defender *rollback.FighterState, seq uint64, chord rollback.StanceChord, judgment rollback.TimingJudgment, move rollback.MoveID) {
	attacker.Frame, defender.Frame = seq, seq
	attacker.Stance = chord
	attacker.LastJudgment = judgment
	attacker.ActiveMove = move
	attacker.MoveStartFrame = seq

	dmg := combat.Damage(move, judgment)
	if !combat.IsDefensive(move) && dmg > 0 {
		defender.Health -= dmg
	}
	if drain := combat.StaminaDrainOnDefender(move); drain > 0 {
		defender.Stamina -= drain
	}
	if judgment == rollback.JudgmentMiss {
		attacker.Stamina -= combat.MissStaminaCost(attacker.Conditioning)
	}
}

// runRollbackDemo simulates discovering, right after resolving ledger
// sequence mispredictSeq locally, that the remote input it used for P2 does
// not match what actually arrived over the network. It reverts the ledger
// to the entry before the mismatch and resimulates that one entry with the
// corrected chord, showing the outcome actually change: P1 took a phantom
// jab from a P2 chord that, corrected, turns out to have been a slip.
func runRollbackDemo(ledger *rollback.Ledger, mispredictSeq uint64, p1, p2 *rollback.FighterState, wrongChord, rightChord rollback.StanceChord) {
	fmt.Println()
	fmt.Printf(red("!! desync: P2's confirmed input for ledger seq %d does not match the local prediction")+"\n", mispredictSeq)
	fmt.Printf("   predicted chord=%s, confirmed chord=%s\n", chordName(wrongChord), chordName(rightChord))
	time.Sleep(sleepBetweenBeats)

	revertSeq := mispredictSeq - 1
	fmt.Printf(yellow("   reverting to ledger seq %d")+"\n", revertSeq)
	time.Sleep(sleepBetweenBeats)

	snap, err := ledger.RevertTo(revertSeq)
	if err != nil {
		fmt.Println(red("   rollback failed: " + err.Error()))
		return
	}
	*p1, *p2 = snap.Fighters[0], snap.Fighters[1]

	fmt.Println(yellow(fmt.Sprintf("   resimulating ledger seq %d with the corrected input", mispredictSeq)))
	// This demo's script keeps ledger sequence and beat number numerically
	// equal (one action per beat), so the beat this entry resolved on is
	// just mispredictSeq itself.
	beatFrame := mispredictSeq * framesPerBeat
	judgment := combat.JudgeTiming(beatFrame, framesPerBeat, perfectWindow, goodWindow)
	correctedMove := combat.Resolve(rightChord, judgment, false)

	// No manual health patch needed here: *p1 was reset above to the
	// snapshot from before the mispredicted jab was ever applied, so
	// resimulating with the corrected (defensive) move naturally never
	// re-applies damage that shouldn't have happened.
	applyMove(p2, p1, mispredictSeq, rightChord, judgment, correctedMove)

	ledger.Record(rollback.FrameSnapshot{
		Frame:     mispredictSeq,
		Fighters:  [2]rollback.FighterState{*p1, *p2},
		Inputs:    inputsFor(beatFrame, beatEvent{fighter: rollback.FighterP2, chord: rightChord}),
		Confirmed: true,
	})

	printBeat(mispredictSeq, rollback.FighterP2, rightChord, judgment, correctedMove, 0, false)
	fmt.Println(green(fmt.Sprintf("   corrected: P1 health back to %d, P2's jab never actually happened", p1.Health)))
	fmt.Println()
}

func inputsFor(beatFrame uint64, ev beatEvent) [2]rollback.InputFrame {
	var inputs [2]rollback.InputFrame
	idx := 0
	if ev.fighter == rollback.FighterP2 {
		idx = 1
	}
	inputs[idx] = rollback.InputFrame{Frame: beatFrame, Fighter: ev.fighter, Chord: ev.chord, ChordReleased: ev.released}
	return inputs
}

func printBeat(beat uint64, fighter rollback.FighterID, chord rollback.StanceChord, judgment rollback.TimingJudgment, move rollback.MoveID, dmg int16, released bool) {
	tag := "P1"
	color := cyan
	if fighter == rollback.FighterP2 {
		tag = "P2"
		color = magenta
	}
	dmgStr := ""
	if dmg > 0 {
		dmgStr = fmt.Sprintf(" dmg %d", dmg)
	}
	rel := ""
	if released {
		rel = " (released)"
	}
	fmt.Printf("[beat %d] %s chord=%-12s%s -> %-8s -> %-10s%s\n",
		beat, color(tag), chordName(chord), rel, judgmentName(judgment), moveName(move), dmgStr)
	time.Sleep(sleepBetweenBeats)
}

func printFighter(f rollback.FighterState) {
	tag := "P1"
	if f.Fighter == rollback.FighterP2 {
		tag = "P2"
	}
	fmt.Printf("  %s  health %3d/100  stamina %3d/100\n", tag, f.Health, f.Stamina)
}

func chordName(c rollback.StanceChord) string {
	if c == 0 {
		return "none"
	}
	s := ""
	if c.Has(rollback.ChordL1) {
		s += "L1+"
	}
	if c.Has(rollback.ChordL2) {
		s += "L2+"
	}
	if c.Has(rollback.ChordR1) {
		s += "R1+"
	}
	if c.Has(rollback.ChordR2) {
		s += "R2+"
	}
	return s[:len(s)-1]
}

func judgmentName(j rollback.TimingJudgment) string {
	switch j {
	case rollback.JudgmentPerfect:
		return "PERFECT"
	case rollback.JudgmentGood:
		return "good"
	case rollback.JudgmentMiss:
		return "miss"
	default:
		return "-"
	}
}

func moveName(m rollback.MoveID) string {
	switch m {
	case rollback.MoveJab:
		return "jab"
	case rollback.MoveTeep:
		return "teep"
	case rollback.MoveLowKick:
		return "low kick"
	case rollback.MoveClinch:
		return "clinch"
	case rollback.MoveSlip:
		return "slip"
	case rollback.MoveFeint:
		return "feint"
	default:
		return "none"
	}
}

// A handful of ANSI helpers so the recorded terminal demo reads clearly.
// Kept deliberately minimal, no external TUI dependency for a scripted,
// non-interactive program.
func wrap(code string) func(string) string {
	return func(s string) string { return "\x1b[" + code + "m" + s + "\x1b[0m" }
}

var (
	bold    = wrap("1")
	dim     = wrap("2")
	red     = wrap("31")
	green   = wrap("32")
	yellow  = wrap("33")
	cyan    = wrap("36")
	magenta = wrap("35")
)

// sleepBetweenBeats paces the recorded demo so it reads like a live match
// instead of dumping every line instantly.
var sleepBetweenBeats = 550 * time.Millisecond
