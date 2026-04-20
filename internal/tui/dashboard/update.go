package dashboard

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/table"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		return m.handleResize(msg)
	case tickMsg:
		return m.handleTick()
	case vmStatusMsg:
		m.vmRunning = msg.running
		m.vmStatusAt = time.Now()
		return m, nil
	case psResultMsg:
		m.jobs = msg.jobs
		m.lastError = msg.err
		m.updateTableRows()
		return m, nil
	case healthResultMsg:
		m.health.OK = msg.ok
		m.health.Latency = msg.latency
		m.health.Err = msg.err
		if msg.err != nil {
			m.lastError = msg.err
		}
		return m, nil
	case logChunkMsg:
		m.logsContent += string(msg.data)
		return m, nil
	case logResultMsg:
		m.logsContent = msg.content
		m.lastError = msg.err
		return m, nil
	case logStreamStartedMsg:
		m.logStreamMu.Lock()
		if m.logStreamCancel != nil {
			m.logStreamCancel()
		}
		m.logStreamCancel = msg.cancel
		m.logStreamMu.Unlock()
		return m, nil
	case logStreamDoneMsg:
		m.logStreamMu.Lock()
		m.logStreamCancel = nil
		m.logStreamMu.Unlock()
		return m, nil
	case startResultMsg:
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
			m.view = ViewJobsList
			m.cmdInput.SetValue("")
			m.mountInput.SetValue(".:/workspace")
			m.focusIdx = 0
			return m, fetchPsCmd(m.client)
		}
		return m, nil
	case errorMsg:
		m.lastError = msg.err
		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewJobsList:
		return m.handleJobsListKey(msg)
	case ViewLogs:
		return m.handleLogsKey(msg)
	case ViewStartModal:
		return m.handleStartModalKey(msg)
	}
	return m, nil
}

func (m *Model) handleJobsListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		interval := time.Duration(m.refreshMs) * time.Millisecond
		return m, tea.Batch(
			tickCmd(interval),
			fetchVmStatusCmd(),
			fetchPsCmd(m.client),
			fetchHealthCmd(m.client),
		)
	case "enter":
		if len(m.jobs) == 0 {
			return m, nil
		}
		idx := m.tableModel.Cursor()
		if idx >= len(m.jobs) {
			return m, nil
		}
		m.logsJobID = m.jobs[idx].JobID
		m.logsContent = ""
		m.logsFollow = false
		m.view = ViewLogs
		return m, fetchLogsCmd(m.client, m.logsJobID)
	case "s":
		if len(m.jobs) == 0 {
			return m, nil
		}
		idx := m.tableModel.Cursor()
		if idx >= len(m.jobs) {
			return m, nil
		}
		jobID := m.jobs[idx].JobID
		if m.jobs[idx].Status == "running" {
			return m, tea.Batch(stopJobCmd(m.client, jobID), fetchPsCmd(m.client))
		}
		return m, nil
	case "a":
		m.view = ViewStartModal
		m.focusIdx = 0
		m.lastError = nil
		m.cmdInput.Focus()
		m.mountInput.Blur()
		return m, nil
	case "d":
		return m, tea.Batch(downCmd(m.client), fetchPsCmd(m.client))
	case "v":
		m.showDebug = !m.showDebug
		return m, nil
	}
	var cmd tea.Cmd
	m.tableModel, cmd = m.tableModel.Update(msg)
	return m, cmd
}

func (m *Model) handleLogsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.cancelLogStream()
		m.view = ViewJobsList
		return m, nil
	case "f":
		m.logsFollow = !m.logsFollow
		if m.logsFollow && m.program != nil {
			m.logsContent = "" // stream includes history
			return m, streamLogsCmd(m.client, m.logsJobID, m.program.Send)
		}
		m.cancelLogStream()
		return m, nil
	}
	return m, nil
}

func (m *Model) cancelLogStream() {
	m.logStreamMu.Lock()
	if m.logStreamCancel != nil {
		m.logStreamCancel()
		m.logStreamCancel = nil
	}
	m.logStreamMu.Unlock()
}

func (m *Model) handleStartModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = ViewJobsList
		m.cmdInput.SetValue("")
		m.mountInput.SetValue(".:/workspace")
		return m, nil
	case "enter":
		if m.focusIdx == 0 {
			m.focusIdx = 1
			m.cmdInput.Blur()
			m.mountInput.Focus()
			return m, nil
		}
		cmdStr := m.cmdInput.Value()
		mountStr := m.mountInput.Value()
		if mountStr == "" {
			mountStr = ".:/workspace"
		}
		m.view = ViewJobsList
		return m, startJobCmd(m.client, cmdStr, mountStr)
	case "tab":
		// toggle focus
		if m.focusIdx == 0 {
			m.focusIdx = 1
			m.cmdInput.Blur()
			m.mountInput.Focus()
		} else {
			m.focusIdx = 0
			m.mountInput.Blur()
			m.cmdInput.Focus()
		}
		return m, nil
	}
	if m.focusIdx == 0 {
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.mountInput, cmd = m.mountInput.Update(msg)
	return m, cmd
}

func (m *Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.tableModel.SetWidth(msg.Width - 4)
	return m, nil
}

func (m *Model) handleTick() (tea.Model, tea.Cmd) {
	interval := time.Duration(m.refreshMs) * time.Millisecond
	cmds := []tea.Cmd{tickCmd(interval), fetchPsCmd(m.client), fetchHealthCmd(m.client)}
	if time.Since(m.vmStatusAt) >= vmStatusRefreshInterval {
		cmds = append(cmds, fetchVmStatusCmd())
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) updateTableRows() {
	now := time.Now().Unix()
	var rows []table.Row
	for _, j := range m.jobs {
		uptime := formatUptime(j.StartedAt, j.FinishedAt, now)
		cmdStr := strings.Join(j.Cmd, " ")
		if len(cmdStr) > 37 {
			cmdStr = cmdStr[:34] + "..."
		}
		rows = append(rows, table.Row{j.JobID, j.Status, uptime, cmdStr})
	}
	m.tableModel.SetRows(rows)
}
