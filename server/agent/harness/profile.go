package harness

import (
	"regexp"
	"strings"
)

const (
	MaxStepsNormal   = 8
	MaxStepsReactor  = 3
	MaxStepsReadOnly = 5
	MaxStepsWorker   = 64
)

type Profile int

const (
	ProfileNormal Profile = iota
	ProfileReactor
	ProfileReactorReadOnly
	ProfileWorker
)

func (p Profile) String() string {
	switch p {
	case ProfileReactor:
		return "reactor"
	case ProfileReactorReadOnly:
		return "reactor_readonly"
	case ProfileWorker:
		return "worker"
	default:
		return "normal"
	}
}

func (p Profile) IsReactor() bool {
	return p == ProfileReactor || p == ProfileReactorReadOnly
}

func (p Profile) valid() bool {
	return p >= ProfileNormal && p <= ProfileWorker
}

func (p Profile) MaxSteps(hasDestructive bool) int {
	switch p {
	case ProfileReactor:
		return MaxStepsReactor
	case ProfileReactorReadOnly:
		if hasDestructive {
			return MaxStepsReactor
		}
		return MaxStepsReadOnly
	case ProfileWorker:
		return MaxStepsWorker
	default:
		return MaxStepsNormal
	}
}

var untrustedTagPattern = regexp.MustCompile(`(?i)<(/?)\s*untrusted`)

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

func NewReactorRunner(c Completer, exec ToolExecutor, opts ...RunnerOption) (Runner, error) {
	combined := make([]RunnerOption, 0, len(opts)+1)
	combined = append(combined, opts...)
	combined = append(combined, WithProfile(ProfileReactor))
	return NewRunner(c, exec, combined...)
}
