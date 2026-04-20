package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) View() string {
	switch m.view {
	case ViewJobsList:
		return m.viewJobsList()
	case ViewLogs:
		return m.viewLogs()
	case ViewStartModal:
		return m.viewStartModal()
	}
	return ""
}

func (m *Model) viewJobsList() string {
	var b strings.Builder

	// Header
	vmState := "stopped"
	if m.vmRunning {
		vmState = "running"
	}
	agentState := "fail"
	if m.health.OK {
		agentState = "ok"
	}
	latencyStr := "-"
	if m.health.OK {
		latencyStr = fmt.Sprintf("%dms", m.health.Latency.Milliseconds())
	}
	now := time.Now().Format("15:04:05")
	header := fmt.Sprintf("VM: %s | Agent: %s | Latency: %s | %s",
		vmState, agentState, latencyStr, now)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n\n")

	// Table
	b.WriteString(m.tableModel.View())
	b.WriteString("\n\n")

	// Status line
	if m.lastError != nil {
		b.WriteString(statusErrStyle.Render("Error: " + m.lastError.Error()))
		b.WriteString("\n")
	}

	// Footer
	footer := "r: refresh | enter: logs | s: stop | a: start | d: down | v: debug | q: quit"
	b.WriteString(footerStyle.Render(footer))
	b.WriteString("\n")

	if m.showDebug {
		b.WriteString("\n--- debug ---\n")
		b.WriteString(fmt.Sprintf("jobs=%d vm=%v health=%v\n", len(m.jobs), m.vmRunning, m.health.OK))
	}

	return b.String()
}

func (m *Model) viewLogs() string {
	var b strings.Builder

	title := fmt.Sprintf("Logs: %s", m.logsJobID)
	if m.logsFollow {
		title += " (following)"
	}
	b.WriteString(headerStyle.Render(title))
	b.WriteString("\n\n")

	content := m.logsContent
	if content == "" && !m.logsFollow {
		content = "(no logs yet)"
	}
	// Truncate if very long for display
	lines := strings.Split(content, "\n")
	maxLines := 30
	if len(lines) > maxLines && !m.logsFollow {
		lines = lines[len(lines)-maxLines:]
		content = strings.Join(lines, "\n")
	}
	b.WriteString(lipgloss.NewStyle().Width(80).Render(content))
	b.WriteString("\n\n")

	footer := "f: toggle follow | esc: back"
	b.WriteString(footerStyle.Render(footer))

	return b.String()
}

func (m *Model) viewStartModal() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Start new job"))
	b.WriteString("\n\n")

	b.WriteString("Command: ")
	b.WriteString(m.cmdInput.View())
	b.WriteString("\n")

	b.WriteString("Mount (default .:/workspace): ")
	b.WriteString(m.mountInput.View())
	b.WriteString("\n\n")

	if m.lastError != nil {
		b.WriteString(statusErrStyle.Render("Error: " + m.lastError.Error()))
		b.WriteString("\n")
	}

	b.WriteString(footerStyle.Render("enter: next/submit | tab: switch field | esc: cancel"))

	return b.String()
}
