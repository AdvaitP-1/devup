package dashboard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"devup/internal/api"
	"devup/internal/client"
	"devup/internal/config"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	vmStatusRefreshInterval = 5 * time.Second
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
	statusErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

type Model struct {
	// Config
	client     *client.Client
	configPath string
	refreshMs  int

	// View state
	view       int
	verbose    bool
	showDebug  bool

	// Jobs list
	jobs       []api.JobInfo
	selected   int
	tableModel table.Model

	// VM/Agent status (cached, async)
	vmRunning     bool
	vmStatusAt    time.Time
	health        HealthState
	lastError     error

	// Logs view
	logsContent   string
	logsJobID     string
	logsFollow    bool
	logStreamCancel context.CancelFunc
	logStreamMu    sync.Mutex

	// Start modal
	cmdInput   textinput.Model
	mountInput textinput.Model
	focusIdx   int

	// Program ref for log streaming
	program *tea.Program
}

func NewModel(cfg *config.Config, configPath string, refreshMs int, verbose bool) *Model {
	cmdInput := textinput.New()
	cmdInput.Placeholder = "python3 -m http.server 8000"
	cmdInput.Width = 50

	mountInput := textinput.New()
	mountInput.Placeholder = ".:/workspace"
	mountInput.SetValue(".:/workspace")
	mountInput.Width = 50

	columns := []table.Column{
		{Title: "JOB_ID", Width: 10},
		{Title: "STATUS", Width: 10},
		{Title: "UPTIME", Width: 10},
		{Title: "CMD", Width: 40},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	return &Model{
		client:     client.New(cfg.Token),
		configPath: configPath,
		refreshMs:  refreshMs,
		verbose:    verbose,
		jobs:       nil,
		selected:   0,
		tableModel: t,
		view:       ViewJobsList,
		cmdInput:   cmdInput,
		mountInput: mountInput,
		focusIdx:   0,
	}
}

func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *Model) Init() tea.Cmd {
	interval := time.Duration(m.refreshMs) * time.Millisecond
	return tea.Batch(
		tickCmd(interval),
		fetchVmStatusCmd(),
		fetchPsCmd(m.client),
		fetchHealthCmd(m.client),
	)
}

func formatUptime(started, finished, now int64) string {
	end := now
	if finished > 0 {
		end = finished
	}
	sec := end - started
	if sec < 0 {
		return "0s"
	}
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm%ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh%dm", sec/3600, (sec%3600)/60)
}
