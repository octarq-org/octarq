// Package clitop hosts the terminal consoles: `octarq top` (live daemon
// vitals over IPC) and `octarq setup` (interactive first-boot wizard). Both
// degrade honestly: top prints one plain-text snapshot when stdout is not a
// TTY, and setup refuses to run without a TTY instead of guessing answers.
package clitop

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/octarq-org/octarq/server/internal/ipc"
)

// Snapshot is one refresh of daemon state for the top console.
type Snapshot struct {
	Status  *ipc.StatusReply
	Metrics *ipc.MetricsReply
	Err     string
}

// FetchSnapshot reads status + metrics from the daemon. A dead daemon yields
// Err — never fabricated vitals.
func FetchSnapshot(ctx context.Context, socketPath string) Snapshot {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	c := ipc.NewClient(socketPath)
	status, err := c.Status(ctx)
	if err != nil {
		return Snapshot{Err: err.Error()}
	}
	snap := Snapshot{Status: status}
	if metrics, err := c.Metrics(ctx); err == nil {
		snap.Metrics = metrics
	}
	return snap
}

func formatUptime(secs int64) string {
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, int(d.Hours())%24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	valueStyle = lipgloss.NewStyle().Bold(true)
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

// RenderPlain renders one snapshot as plain text for pipes, CI and logs.
func RenderPlain(s Snapshot) string {
	var b strings.Builder
	b.WriteString("octarq top\n")
	if s.Err != "" {
		fmt.Fprintf(&b, "daemon unreachable: %s\n", s.Err)
		return b.String()
	}
	st := s.Status
	fmt.Fprintf(&b, "version: %s  uptime: %s  pid: %d\n", st.Version, formatUptime(st.UptimeSecs), st.PID)
	fmt.Fprintf(&b, "goroutines: %d  mem: %s\n", st.Goroutines, formatBytes(st.MemAlloc))
	if s.Metrics != nil {
		fmt.Fprintf(&b, "overall: %s\n", s.Metrics.Overall)
		for _, p := range s.Metrics.Providers {
			line := fmt.Sprintf("  %-24s %s", p.Name, p.Status)
			if p.Detail != "" {
				line += "  " + p.Detail
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

type tickMsg struct{}
type snapMsg struct{ snap Snapshot }

type topModel struct {
	socket string
	snap   Snapshot
	ready  bool
}

func (m topModel) Init() tea.Cmd {
	return tea.Batch(fetchCmd(m.socket), tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickMsg{} }))
}

func fetchCmd(socket string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return snapMsg{snap: FetchSnapshot(ctx, socket)}
	}
}

func (m topModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		return m, tea.Batch(fetchCmd(m.socket), tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickMsg{} }))
	case snapMsg:
		m.snap = msg.snap
		m.ready = true
	}
	return m, nil
}

func (m topModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("octarq top") + "  " + labelStyle.Render("q to quit") + "\n\n")
	if !m.ready {
		b.WriteString("connecting to daemon…\n")
		return b.String()
	}
	s := m.snap
	if s.Err != "" {
		b.WriteString(errStyle.Render("daemon unreachable: "+s.Err) + "\n")
		return b.String()
	}
	st := s.Status
	fmt.Fprintf(&b, "%s %s   %s %s   %s %d\n",
		labelStyle.Render("version:"), valueStyle.Render(st.Version),
		labelStyle.Render("uptime:"), valueStyle.Render(formatUptime(st.UptimeSecs)),
		labelStyle.Render("pid:"), st.PID)
	fmt.Fprintf(&b, "%s %s   %s %s\n\n",
		labelStyle.Render("goroutines:"), valueStyle.Render(fmt.Sprint(st.Goroutines)),
		labelStyle.Render("mem:"), valueStyle.Render(formatBytes(st.MemAlloc)))
	if s.Metrics != nil {
		overall := s.Metrics.Overall
		styled := valueStyle.Render(overall)
		if overall == "healthy" {
			styled = okStyle.Render(overall)
		}
		b.WriteString(labelStyle.Render("overall: ") + styled + "\n")
		for _, p := range s.Metrics.Providers {
			status := p.Status
			if status == "healthy" {
				status = okStyle.Render(status)
			} else if status != "" {
				status = errStyle.Render(status)
			}
			line := fmt.Sprintf("  %-24s %s", p.Name, status)
			if p.Detail != "" {
				line += "  " + labelStyle.Render(p.Detail)
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// isTTY reports whether w is a character-device stdout.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// RunTop runs the live console on a TTY, or prints one plain-text snapshot
// and exits when stdout is piped (CI-safe, exit 0 on snapshot, 1 when the
// daemon is unreachable).
func RunTop(ctx context.Context, stdout io.Writer, socketPath string) int {
	if os.Getenv("CI") != "" || !isTTY(stdout) {
		snap := FetchSnapshot(ctx, socketPath)
		fmt.Fprint(stdout, RenderPlain(snap))
		if snap.Err != "" {
			return 1
		}
		return 0
	}
	p := tea.NewProgram(topModel{socket: socketPath}, tea.WithOutput(stdout))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stdout, "top: %v\n", err)
		return 1
	}
	return 0
}
