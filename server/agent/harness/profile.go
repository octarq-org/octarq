package harness

import (
	"regexp"
	"strings"
)

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

// String returns the canonical profile name for tracing and audit logs.
func (p Profile) String() string {
	switch p {
	case ProfileReactor:
		return "reactor"
	case ProfileReactorReadOnly:
		return "reactor_readonly"
	default:
		return "normal"
	}
}

// IsReactor reports whether the profile is an unattended event-driven reactor profile.
func (p Profile) IsReactor() bool {
	return p == ProfileReactor || p == ProfileReactorReadOnly
}

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

var untrustedTagPattern = regexp.MustCompile(`(?i)<(/?)\s*untrusted`)

// FenceUntrustedEvent wraps an external event or run payload inside an <untrusted>
// XML fence, neutralizing any forged closing tags inside the payload so untrusted
// event data cannot break out of the boundary.
func FenceUntrustedEvent(source, content string) string {
	src := strings.TrimSpace(source)
	if src == "" {
		src = "event"
	}
	src = strings.NewReplacer(`"`, `'`, "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;").Replace(src)
	escaped := untrustedTagPattern.ReplaceAllStringFunc(content, func(m string) string {
		if len(m) > 0 && m[0] == '<' {
			return "&lt;" + m[1:]
		}
		return m
	})
	return `<untrusted source="` + src + `" safe_mode="true">` + "\n" + escaped + "\n</untrusted>"
}

// NewReactorRunner constructs a Runner locked to ProfileReactor (MaxStepsReactor = 3).
// ProfileReactor is applied after caller options so the 3-step unattended ceiling
// cannot be raised by an option override.
func NewReactorRunner(c Completer, exec ToolExecutor, opts ...RunnerOption) Runner {
	combined := make([]RunnerOption, 0, len(opts)+1)
	combined = append(combined, opts...)
	combined = append(combined, WithProfile(ProfileReactor))
	return NewRunner(c, exec, combined...)
}
