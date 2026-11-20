package tenantsql

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var validIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const RedactedValue = "[redacted]"

var SensitiveColumns = map[string]bool{
	"password":      true,
	"password_hash": true,
	"hash":          true,
	"config":        true,
	"raw":           true,
	"html":          true,
	"secret":        true,
	"token":         true,
	"access_token":  true,
	"refresh_token": true,
	"private_key":   true,
	"otp":           true,
	"subject":       true,
	"text":          true,
	"storage_key":   true,
	"fingerprint":   true,
	"ciphertext":    true,
}

type TenantColumn struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type TenantView struct {
	Name       string                  `json:"name"`
	Columns    []TenantColumn          `json:"columns"`
	Sensitive  []string                `json:"sensitive"`
	Definition func(orgID uint) string `json:"-"`
}

type Registry struct {
	mu    sync.RWMutex
	views map[string]TenantView
}

func NewRegistry() *Registry {
	return &Registry{
		views: make(map[string]TenantView),
	}
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

func (r *Registry) Register(view TenantView) error {
	name := strings.TrimSpace(view.Name)
	if name == "" {
		return errors.New("view name cannot be empty")
	}
	if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(DefaultViewPrefix)) {
		return fmt.Errorf("view name %q must start with required prefix %q", view.Name, DefaultViewPrefix)
	}
	if !validIdentPattern.MatchString(name) {
		return fmt.Errorf("invalid view name %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", view.Name)
	}
	if view.Definition == nil {
		return fmt.Errorf("view definition func for %q cannot be nil", view.Name)
	}

	key := strings.ToLower(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.views[key]; exists {
		return fmt.Errorf("view %q already registered", view.Name)
	}

	r.views[key] = view
	return nil
}

func (r *Registry) Lookup(name string) (TenantView, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	r.mu.RLock()
	defer r.mu.RUnlock()

	view, ok := r.views[key]
	return view, ok
}

func (r *Registry) List() []TenantView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	views := make([]TenantView, 0, len(r.views))
	for _, v := range r.views {
		views = append(views, v)
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})

	return views
}

func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views = make(map[string]TenantView)
}

func quoteIdent(ident string) (string, error) {
	if !validIdentPattern.MatchString(ident) {
		return "", fmt.Errorf("invalid SQL identifier %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", ident)
	}
	return `"` + ident + `"`, nil
}

func HasScopeColumn(view TenantView) bool {
	for _, col := range view.Columns {
		if strings.EqualFold(strings.TrimSpace(col.Name), "scope_key") {
			return true
		}
	}
	return false
}

func wrapScopedRedactedViewSQL(view TenantView, inner string, allowPersonal bool) (string, error) {
	scopeWhere := ""
	if !allowPersonal && (HasScopeColumn(view) || strings.Contains(strings.ToLower(inner), "scope_key")) {
		scopeWhere = ` WHERE "scope_key" LIKE 'work:%'`
	}

	if len(view.Columns) == 0 {
		return fmt.Sprintf("SELECT * FROM (%s) AS _octarq_tv%s", inner, scopeWhere), nil
	}

	sensitiveSet := make(map[string]bool)
	for _, s := range view.Sensitive {
		sensitiveSet[strings.ToLower(strings.TrimSpace(s))] = true
	}

	selectExprs := make([]string, 0, len(view.Columns))
	for _, col := range view.Columns {
		colName := strings.TrimSpace(col.Name)
		if colName == "" {
			return "", fmt.Errorf("column name in view %q cannot be empty", view.Name)
		}
		quotedCol, err := quoteIdent(colName)
		if err != nil {
			return "", fmt.Errorf("invalid column %q in view %q: %w", colName, view.Name, err)
		}

		colLower := strings.ToLower(colName)
		if sensitiveSet[colLower] || SensitiveColumns[colLower] {
			selectExprs = append(selectExprs, fmt.Sprintf("'%s' AS %s", RedactedValue, quotedCol))
		} else {
			selectExprs = append(selectExprs, quotedCol)
		}
	}

	return fmt.Sprintf("SELECT %s FROM (%s) AS _octarq_tv%s", strings.Join(selectExprs, ", "), inner, scopeWhere), nil
}
