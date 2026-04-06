package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/workmd"
)

type projectsLoadedMsg struct {
	projects []workmd.Project
}

// --- Pages ---

type page int

const (
	pageDashboard page = iota // overview of all projects + task counts
	pageProject               // single project WORK.md viewer/editor
	pageDump                  // brain dump input
	pageScripts               // maintenance scripts
	pageCleanup               // ollama WORK.md cleanup preview
)

// --- Modes ---

type mode int

const (
	modeNormal mode = iota
	modeEdit         // inline editing a section
	modeHelp         // help overlay
	modeConfirm      // confirm action
	modeDumpInput    // typing a brain dump
	modeDumpRouting  // ollama is classifying
	modeDumpConfirm  // showing route result, waiting for y/n
	modeCleanupWait   // ollama is cleaning up
	modeDumpReview    // stepping through routed items
	modeDumpClarify   // asking user to clarify unclear item
	modeDumpSummary   // post-dump summary, esc to dismiss
)

// --- Model ---

type model struct {
	// Layout
	width  int
	height int

	// Navigation
	page   page
	mode   mode
	cursor int // selected project index (dashboard) or section index (project view)

	// Projects
	projects []workmd.Project
	selected int // index of currently viewed project

	// Viewport for markdown rendering
	viewport viewport.Model

	// Inline editing
	editArea textarea.Model
	editSection string // which section is being edited

	// Brain dump
	dumpArea        textarea.Model
	dumpText        string          // raw input text
	dumpItems       []ollama.RouteItem // multi-routed items from ollama
	dumpCursor      int             // which item we're reviewing
	dumpAccepted    int               // count of accepted items
	dumpSkipped     int               // count of skipped items
	dumpSkippedList []ollama.RouteItem // items that were skipped
	dumpClarifyArea textarea.Model    // textarea for clarification input
	dumpResult      string            // last status message for display

	// Cleanup
	cleanupOriginal string // original content before cleanup
	cleanupResult   string // ollama-cleaned content

	// Scripts
	scriptCursor int
	scriptOutput string

	// Spinner
	spinner spinner.Model

	// Loading
	loading bool

	// Status
	statusMsg    string
	statusExpiry time.Time

	// Scroll
	dashScroll      int
	dashRightScroll int
	helpScroll      int
}

func newModel() model {
	dump := textarea.New()
	dump.Placeholder = "brain dump — type an idea, thought, or task... (ctrl+d to route)"
	dump.SetWidth(80)
	dump.SetHeight(12)
	dump.CharLimit = 10000

	edit := textarea.New()
	edit.SetWidth(80)
	edit.SetHeight(20)
	edit.CharLimit = 0 // no limit

	clarify := textarea.New()
	clarify.Placeholder = "which project is this for? (type name or description)"
	clarify.SetWidth(80)
	clarify.SetHeight(3)
	clarify.CharLimit = 500

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6CCA"))

	return model{
		loading:         true,
		dumpArea:        dump,
		dumpClarifyArea: clarify,
		editArea:        edit,
		viewport:        vp,
		spinner:         sp,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("sb"),
		tickCmd(),
		m.spinner.Tick,
		func() tea.Msg {
			return projectsLoadedMsg{projects: workmd.Discover()}
		},
	)
}

// --- Messages ---

type tickMsg time.Time
type statusClearMsg struct{}

type dumpRoutedMsg struct {
	items []ollama.RouteItem
	err   error
}

type dumpReroutedMsg struct {
	item *ollama.RouteItem
	err  error
}

type cleanupDoneMsg struct {
	result string
	err    error
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

