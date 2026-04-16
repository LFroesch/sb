package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/workmd"
)

type chainResult struct {
	name   string
	action string // "accepted", "skipped", "error"
}

type searchMatch struct {
	projectIdx int
	line       string
}

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
	modeTodoWait      // ollama is generating next todo
	modeTodoResult    // showing next todo result
	modeSearch        // fuzzy search across WORK.md content
	modeDumpReview    // stepping through routed items
	modeDumpClarify   // asking user to clarify unclear item
	modeDumpSummary   // post-dump summary, esc to dismiss

	modeChainCleanupWait     // ollama running on current project in chain
	modeChainCleanupReview   // reviewing diff for current project
	modeChainCleanupFeedback // user typing correction hint for regen
	modeChainCleanupSummary  // chain done — show results

	modeCleanupFeedback // single-project cleanup: user typing feedback for regen

	modePlanWait   // ollama generating daily plan
	modePlanResult // showing daily plan result
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
	dumpSkippedList    []ollama.RouteItem // items that were skipped
	dumpClarifyArea   textarea.Model    // textarea for clarification input
	dumpResult        string            // last status message for display
	dumpSummaryScroll int               // scroll offset for summary screen

	// Cleanup
	cleanupOriginal string // original content before cleanup
	cleanupResult   string // ollama-cleaned content

	// Chain cleanup
	chainQueue          []int
	chainCursor         int
	chainAccepted       int
	chainSkipped        int
	chainFeedback       textarea.Model
	chainResults        []chainResult
	chainSummaryScroll  int

	// Project selection (for chain cleanup / plan)
	selectedProjects map[string]bool // keyed by project path

	// Daily plan
	planResult string
	planScroll int

	// Todo
	todoResult string // ollama next-todo response

	// Search
	searchQuery   string
	searchMatches []searchMatch

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

	// Config
	cfg *config.Config

	// Favorites
	favorites map[string]bool

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

	chainFB := textarea.New()
	chainFB.Placeholder = "what needs fixing? (e.g. 'don't merge Updates + Features')"
	chainFB.SetWidth(80)
	chainFB.SetHeight(3)
	chainFB.CharLimit = 500

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6CCA"))

	cfg := config.Load()

	return model{
		loading:          true,
		cfg:              cfg,
		dumpArea:         dump,
		dumpClarifyArea:  clarify,
		chainFeedback:    chainFB,
		editArea:         edit,
		viewport:         vp,
		spinner:          sp,
		selectedProjects: make(map[string]bool),
		favorites:        loadFavorites(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("sb"),
		tickCmd(),
		m.spinner.Tick,
		func() tea.Msg {
			return projectsLoadedMsg{projects: workmd.Discover(
				m.cfg.ExpandedScanDirs(),
				m.cfg.FilePatterns,
				m.cfg.ExpandedIdeaDirs(),
			)}
		},
	)
}

// --- Messages ---

type tickMsg time.Time
type statusClearMsg struct{}

type todoResultMsg struct {
	result string
	err    error
}

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

type planResultMsg struct {
	result string
	err    error
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

