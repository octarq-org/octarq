package auth

import "sync"

type AuthMethod struct {
	ID        string      `json:"id"`
	Label     string      `json:"label"`
	LoginURL  string      `json:"loginUrl"`
	IconKey   string      `json:"iconKey"`
	Available func() bool `json:"-"`
}

var (
	methodsMu sync.RWMutex
	methods   = make(map[string]AuthMethod)
)

func Register(m AuthMethod) {
	if m.ID == "" {
		return
	}
	methodsMu.Lock()
	defer methodsMu.Unlock()
	methods[m.ID] = m
}

func List() []AuthMethod {
	methodsMu.RLock()
	defer methodsMu.RUnlock()

	result := make([]AuthMethod, 0, len(methods))
	for _, m := range methods {
		if m.Available != nil && !m.Available() {
			continue
		}
		result = append(result, m)
	}
	return result
}

func ResetMethodsForTesting() {
	methodsMu.Lock()
	defer methodsMu.Unlock()
	methods = make(map[string]AuthMethod)
}
