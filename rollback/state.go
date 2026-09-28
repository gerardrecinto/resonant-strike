// Package rollback holds the deterministic simulation state for one match
// and the frame ledger used to roll it back and resimulate it.
//
// Everything in this file is a plain value type with fixed-size numeric
// fields, no floats, no pointers, no maps. That is not a style preference,
// it is a correctness requirement: this state is designed to be hashed for
// desync detection and diffed frame to frame across two machines that may
// be a different CPU architecture or Go build from each other (hashing
// itself is not implemented yet, see docs/ARCHITECTURE.md). Floats round
// differently across platforms and compilers; fixed-point integers do not.
package rollback

// FighterID identifies one of the two combatants in a match.
type FighterID uint8

const (
	FighterP1 FighterID = iota
	FighterP2
)

// StanceChord is the set of shoulder buttons and triggers currently held.
// L1, L2, R1, and R2 combine like frets on a guitar: each of the 16
// combinations (including none held) maps to one physical guard, footwork
// angle, or set of exposed lines. The chord is a bitmask so combining and
// checking it is a single AND, no branching on discrete stance enums.
type StanceChord uint8

const (
	ChordL1 StanceChord = 1 << iota
	ChordL2
	ChordR1
	ChordR2
)

// Has reports whether every bit in want is set in c.
func (c StanceChord) Has(want StanceChord) bool {
	return c&want == want
}

// TimingJudgment classifies how close an executed flick landed to the beat.
// This is computed by the simulation from InputFrame plus the audio clock,
// it is never part of the transmitted input itself.
type TimingJudgment uint8

const (
	JudgmentNone TimingJudgment = iota
	JudgmentPerfect
	JudgmentGood
	JudgmentMiss
)

// MoveID identifies a resolved Muay Thai action. Which MoveID a given
// StanceChord plus TimingJudgment resolves to is table-driven, defined
// outside this package; this type is just the result slot in FighterState.
type MoveID uint16

const (
	MoveNone MoveID = iota
	MoveSlip
	MoveFeint
	MoveJab
	MoveTeep
	MoveLowKick
	MoveClinch
)

// InputFrame is one player's raw input sample for a single simulation tick.
// It is the only thing that ever crosses the network in this scheme: peers
// exchange InputFrame, never FighterState. State is always a pure function
// of the input history, which is what makes rollback possible, resimulating
// a frame means replaying classify-and-apply over InputFrame again, not
// patching a previously computed state.
type InputFrame struct {
	Frame   uint64
	Fighter FighterID
	Chord   StanceChord

	// StickX and StickY are the right thumbstick deflection, the execution
	// trigger, in fixed-point units where 1000 is full deflection. Recorded
	// raw; the simulation judges beat proximity, not the input layer.
	StickX int16
	StickY int16

	// ChordReleased marks the frame the chord was let go on. Releasing a
	// stance in the feint window (defined by the fight's BPM table, not by
	// this struct) resolves to MoveFeint instead of whatever the held chord
	// would have executed on-beat.
	ChordReleased bool
}

// ConditioningSnapshot is the training-camp meta-game state carried into a
// match. It is captured once at match start; only Stamina in FighterState
// reflects in-match depletion, the camp stats themselves do not change
// mid-fight.
type ConditioningSnapshot struct {
	SquatMaxLb    uint16
	DeadliftMaxLb uint16

	// CardioVO2Max is ml/kg/min at one decimal place of fixed-point
	// precision, so 452 means 45.2.
	CardioVO2Max uint16

	// MissPenaltyScale is the stamina penalty multiplier for a missed
	// rhythm input, in basis points where 10000 is the baseline 1.0x. It
	// scales up with SquatMaxLb and DeadliftMaxLb: heavier kinetic load
	// unlocks harder strikes on-beat and costs more stamina off-beat.
	MissPenaltyScale uint16
}

// FighterState is one fighter's complete simulation state at a single
// frame, fully derived from the input history up to and including this
// frame. Nothing in this struct is ever assigned from outside the
// simulation step.
type FighterState struct {
	Frame   uint64
	Fighter FighterID

	// PositionX is fixed-point, 1000 units per meter.
	PositionX int32

	Stance StanceChord
	// StanceHeldSince is the frame the current Stance was first held,
	// needed to evaluate whether a release lands inside the feint window.
	StanceHeldSince uint64

	Health  int16
	Stamina int16

	LastJudgment   TimingJudgment
	ActiveMove     MoveID
	MoveStartFrame uint64

	Conditioning ConditioningSnapshot
}

// FrameSnapshot is the ledger's unit of storage: both fighters' state and
// the inputs that produced it, everything needed to resume simulation from
// this exact point without replaying anything earlier.
type FrameSnapshot struct {
	Frame    uint64
	Fighters [2]FighterState
	Inputs   [2]InputFrame

	// Confirmed is false while an opponent's input for this frame is still
	// a local prediction rather than the value that actually arrived over
	// the network. A later mismatch on a Confirmed=false frame is what
	// triggers RevertTo.
	Confirmed bool
}
