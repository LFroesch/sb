package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LFroesch/sb/internal/workmd"
)

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
	modeCleanupWait  // ollama is cleaning up
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
	dumpArea    textarea.Model
	dumpText    string
	dumpRoute   string // ollama's suggested project
	dumpSection string // ollama's suggested section
	dumpResult  string // last route result message for display

	// Cleanup
	cleanupOriginal string // original content before cleanup
	cleanupResult   string // ollama-cleaned content

	// Scripts
	scriptCursor int
	scriptOutput string

	// Status
	statusMsg    string
	statusExpiry time.Time

	// Scroll
	dashScroll      int
	dashRightScroll int
	helpScroll      int
}

func newModel(projects []workmd.Project) model {
	dump := textarea.New()
	dump.Placeholder = "brain dump — type an idea, thought, or task... (ctrl+d to route)"
	dump.SetWidth(80)
	dump.SetHeight(12)
	dump.CharLimit = 10000

	edit := textarea.New()
	edit.SetWidth(80)
	edit.SetHeight(20)
	edit.CharLimit = 0 // no limit

	vp := viewport.New(80, 20)

	return model{
		projects: projects,
		dumpArea: dump,
		editArea: edit,
		viewport: vp,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("sb"),
		tickCmd(),
	)
}

// --- Messages ---

type tickMsg time.Time
type statusClearMsg struct{}

type dumpRoutedMsg struct {
	project string
	section string
	err     error
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

