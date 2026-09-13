package harness

// Step-count ceilings — design invariant #6 quantitative values.
const (
	MaxStepsNormal   = 8
	MaxStepsReactor  = 3 // destructive reactor hard cap, cannot be relaxed
	MaxStepsReadOnly = 5 // read-only reactor cap, ≤ Normal
)

// Profile selects which step budget applies to a session.
type Profile int

const (
	ProfileNormal          Profile = iota // interactive multi-turn session
	ProfileReactor                        // event-driven unattended session
	ProfileReactorReadOnly                // read-only reactor (no writes)
)

// MaxSteps returns the step ceiling for this profile. When hasDestructive is
// true the reactor profile is locked to MaxStepsReactor regardless of the
// read-only hint.
func (p Profile) MaxSteps(hasDestructive bool) int {
	switch p {
	case ProfileReactor:
		return MaxStepsReactor
	case ProfileReactorReadOnly:
		if hasDestructive {
			return MaxStepsReactor
		}
		return MaxStepsReadOnly
	default: // ProfileNormal
		return MaxStepsNormal
	}
}
