package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors — consistent palette (matches tui-hub apps)
	colorPrimary  = lipgloss.Color("#5AF78E")
	colorAccent   = lipgloss.Color("#57C7FF")
	colorCursorBg = lipgloss.Color("#2a2a40") // subtle bg for selected row
	colorWarn     = lipgloss.Color("#FF5555") // red — bugs/errors
	colorDim      = lipgloss.Color("#606060")
	colorText     = lipgloss.Color("#EEEEEE")
	colorYellow   = lipgloss.Color("#F3F99D")
	colorBacklog  = lipgloss.Color("#5E8DD6") // blue — backlog

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	accentStyle = lipgloss.NewStyle().
			Foreground(colorAccent)

	primaryStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	warnStyle = lipgloss.NewStyle().
			Foreground(colorWarn)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	backlogStyle = lipgloss.NewStyle().
			Foreground(colorBacklog)

	currentStyle = lipgloss.NewStyle().
			Foreground(colorYellow) // yellow — current task counts

	textStyle = lipgloss.NewStyle().
			Foreground(colorText)

	statusStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	keyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	actionStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	panelActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)
)
