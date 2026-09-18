package clitop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/octarq-org/octarq/server/config"
)

// setupField is one wizard prompt.
type setupField struct {
	key      string
	label    string
	def      string
	secret   bool
	generate bool
}

func setupFields() []setupField {
	return []setupField{
		{key: "OCTARQ_ADMIN_USER", label: "Admin username", def: "admin"},
		{key: "OCTARQ_ADMIN_PASSWORD", label: "Admin password", secret: true},
		{key: "OCTARQ_DB_DRIVER", label: "DB driver (sqlite/postgres/mysql)", def: "sqlite"},
		{key: "OCTARQ_DB_DSN", label: "DB DSN (file path or connection string)", def: "octarq.db"},
		{key: "OCTARQ_LISTEN", label: "Listen address", def: ":8080"},
		{key: "OCTARQ_SECRET_KEY", label: "Secret key (empty = generate)", generate: true, secret: true},
	}
}

type setupModel struct {
	fields []setupField
	values map[string]string
	input  []rune
	index  int
	done   bool
	err    string
	force  bool
}

func (m setupModel) Init() tea.Cmd { return nil }

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		return m, tea.Quit
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.err = "setup aborted"
		m.done = true
		return m, tea.Quit
	case tea.KeyEnter:
		m.values[m.fields[m.index].key] = strings.TrimSpace(string(m.input))
		m.input = nil
		m.index++
		if m.index >= len(m.fields) {
			m.done = true
			return m, tea.Quit
		}
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	case tea.KeyRunes:
		m.input = append(m.input, key.Runes...)
	}
	return m, nil
}

func (m setupModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("octarq setup") + "  " + labelStyle.Render("enter to confirm · esc to abort") + "\n\n")
	for i, f := range m.fields {
		marker := "  "
		if i == m.index {
			marker = "> "
		}
		val := m.values[f.key]
		if val == "" && f.def != "" {
			val = labelStyle.Render("[" + f.def + "]")
		} else if f.secret && val != "" {
			val = strings.Repeat("•", len(val))
		}
		b.WriteString(marker + f.label + ": " + val + "\n")
	}
	if m.index < len(m.fields) {
		f := m.fields[m.index]
		shown := string(m.input)
		if f.secret {
			shown = strings.Repeat("•", len(m.input))
		}
		hint := ""
		if f.generate {
			hint = " " + labelStyle.Render("(empty generates a random key)")
		}
		b.WriteString("\n" + f.label + hint + ": " + shown + "█\n")
	}
	return b.String()
}

// generateSecret mints a 32-byte hex secret.
func generateSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// ValidateSetup applies wizard answers without touching disk: empty answers
// fall back to defaults, then the layered loader enforces every Fail-Closed
// rule (driver allowlist, secret floor, required password).
func ValidateSetup(answers map[string]string) (map[string]string, error) {
	resolved := map[string]string{}
	for _, f := range setupFields() {
		v := strings.TrimSpace(answers[f.key])
		if v == "" {
			v = f.def
		}
		resolved[f.key] = v
	}
	if resolved["OCTARQ_SECRET_KEY"] == "" {
		secret, err := generateSecret()
		if err != nil {
			return nil, err
		}
		resolved["OCTARQ_SECRET_KEY"] = secret
	}
	if _, err := config.LoadWithOptions(config.LoadOptions{DotEnvPath: "", Overrides: resolved}); err != nil {
		return nil, err
	}
	return resolved, nil
}

// WriteEnvFile writes resolved answers to path (0600). An existing non-empty
// file fails closed unless force is set.
func WriteEnvFile(path string, resolved map[string]string, force bool) error {
	if !force {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return fmt.Errorf("setup: %s already exists (re-run with --force to overwrite)", path)
		}
	}
	var b strings.Builder
	for _, f := range setupFields() {
		fmt.Fprintf(&b, "%s=%s\n", f.key, resolved[f.key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("setup: write %s: %w", path, err)
	}
	return nil
}

// RunSetup drives the interactive wizard. It requires a TTY and refuses to
// overwrite an existing .env without --force.
func RunSetup(ctx context.Context, stdout io.Writer, envPath string, force bool) int {
	if os.Getenv("CI") != "" || !isTTY(stdout) {
		fmt.Fprintln(stdout, "setup: interactive wizard requires a TTY")
		return 1
	}
	m := setupModel{fields: setupFields(), values: map[string]string{}}
	p := tea.NewProgram(m, tea.WithOutput(stdout))
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(stdout, "setup: %v\n", err)
		return 1
	}
	fm, ok := final.(setupModel)
	if !ok || fm.err != "" {
		fmt.Fprintf(stdout, "setup: %s\n", fm.err)
		return 1
	}
	answers := map[string]string{}
	for _, f := range fm.fields {
		answers[f.key] = fm.values[f.key]
	}
	resolved, err := ValidateSetup(answers)
	if err != nil {
		fmt.Fprintf(stdout, "setup: invalid configuration: %v\n", err)
		return 1
	}
	if err := WriteEnvFile(envPath, resolved, force); err != nil {
		fmt.Fprintf(stdout, "%v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (0600). Start with: octarq server\n", envPath)
	return 0
}
