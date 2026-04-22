package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LFroesch/sb/internal/cockpit"
	"github.com/LFroesch/sb/internal/config"
	"github.com/LFroesch/sb/internal/llm"
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
	pageCleanup               // LLM WORK.md cleanup preview
	pageAgent                 // agent cockpit: job list + launch + attached view
)

// --- Modes ---

type mode int

const (
	modeNormal      mode = iota
	modeEdit             // inline editing a section
	modeHelp             // help overlay
	modeConfirm          // confirm action
	modeDumpInput        // typing a brain dump
	modeDumpRouting      // LLM is classifying
	modeDumpConfirm      // showing route result, waiting for y/n
	modeCleanupWait      // LLM is cleaning up
	modeTodoWait         // LLM is generating next todo
	modeTodoResult       // showing next todo result
	modeSearch           // fuzzy search across WORK.md content
	modeDumpReview       // stepping through routed items
	modeDumpClarify      // asking user to clarify unclear item
	modeDumpSummary      // post-dump summary, esc to dismiss

	modeChainCleanupWait     // LLM running on current project in chain
	modeChainCleanupReview   // reviewing diff for current project
	modeChainCleanupFeedback // user typing correction hint for regen
	modeChainCleanupSummary  // chain done — show results

	modeCleanupFeedback // single-project cleanup: user typing feedback for regen

	modePlanWait   // LLM generating daily plan
	modePlanResult // showing daily plan result

	modeAgentList     // agent tab: job list (default)
	modeAgentPicker   // agent tab: pick file + tasks
	modeAgentLaunch   // agent tab: preset + brief confirm
	modeAgentAttached // agent tab: attached transcript + input
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
	editArea    textarea.Model
	editSection string // which section is being edited

	// Brain dump
	dumpArea          textarea.Model
	dumpText          string          // raw input text
	dumpItems         []llm.RouteItem // multi-routed items from the active LLM
	dumpCursor        int             // which item we're reviewing
	dumpAccepted      int             // count of accepted items
	dumpSkipped       int             // count of skipped items
	dumpSkippedList   []llm.RouteItem // items that were skipped
	dumpClarifyArea   textarea.Model  // textarea for clarification input
	dumpResult        string          // last status message for display
	dumpSummaryScroll int             // scroll offset for summary screen

	// Cleanup
	cleanupOriginal string // original content before cleanup
	cleanupResult   string // LLM-cleaned content

	// Chain cleanup
	chainQueue         []int
	chainCursor        int
	chainAccepted      int
	chainSkipped       int
	chainFeedback      textarea.Model
	chainResults       []chainResult
	chainSummaryScroll int

	// Project selection (for chain cleanup / plan)
	selectedProjects map[string]bool // keyed by project path

	// Daily plan
	planResult string
	planScroll int

	// Todo
	todoResult string // LLM next-todo response

	// Search
	searchQuery   string
	searchMatches []searchMatch

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

	// Agent cockpit
	cockpitClient     cockpit.Client
	cockpitPresets    []cockpit.LaunchPreset
	cockpitProviders  []cockpit.ProviderProfile
	cockpitPaths      cockpit.Paths
	cockpitJobs       []cockpit.Job
	cockpitEvents     <-chan cockpit.Event
	cockpitErr        string
	cockpitMode       string // "daemon" | "in-proc"
	cockpitDetachQuit bool
	agentFilter       string // "all" | "live" | "running" | "attention" | "done"
	agentCursor       int
	pickerFile        string
	pickerProject     string
	pickerRepo        string
	pickerItems       []cockpit.PickerItem
	pickerSelected    map[int]bool
	launchSources     []cockpit.SourceTask
	launchRepo        string
	launchPresetIdx   int
	launchProviderIdx int // 0 = preset default, 1..n = providers[idx-1]
	launchBrief       textarea.Model
	launchFocus       int // 0=preset 1=provider 2=brief
	attachedJobID     cockpit.JobID
	attachedInput     textarea.Model
	attachedFocus     int    // 0=transcript (shortcuts + scroll), 1=input (typing)
	transcriptBuf     string // live assistant output for the in-flight turn
	attachedTurns     []cockpit.Turn

	// Agent confirmation state: when active, next y/n answers the prompt.
	agentConfirmActive bool
	agentConfirmKind   string
	agentConfirmTarget cockpit.JobID
}

func newModel(cfg *config.Config) model {
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

	launchBrief := textarea.New()
	launchBrief.Placeholder = "additional context for the agent (optional)"
	launchBrief.SetWidth(80)
	launchBrief.SetHeight(6)
	launchBrief.CharLimit = 0

	attachedInput := textarea.New()
	attachedInput.Placeholder = "send to agent…"
	attachedInput.SetWidth(80)
	attachedInput.SetHeight(3)
	attachedInput.CharLimit = 0

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C6CCA"))

	return model{
		loading:          true,
		cfg:              cfg,
		dumpArea:         dump,
		dumpClarifyArea:  clarify,
		chainFeedback:    chainFB,
		launchBrief:      launchBrief,
		attachedInput:    attachedInput,
		editArea:         edit,
		viewport:         vp,
		spinner:          sp,
		selectedProjects: make(map[string]bool),
		pickerSelected:   make(map[int]bool),
		favorites:        loadFavorites(),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tea.SetWindowTitle("sb"),
		tickCmd(),
		m.spinner.Tick,
	}
	if m.cockpitEvents != nil {
		cmds = append(cmds, cockpitWatchCmd(m.cockpitEvents))
	}
	cmds = append(cmds, func() tea.Msg {
		projects := workmd.Discover(
			m.cfg.ExpandedScanRoots(),
			m.cfg.FilePatterns,
			m.cfg.ExpandedIdeaDirs(),
			m.cfg,
		)
		// Best-effort: regenerate the routing-context index. Failure here
		// (e.g. read-only $HOME) shouldn't block startup.
		var targets []workmd.SpecialTarget
		if t := m.cfg.CatchallTarget; t != nil {
			targets = append(targets, workmd.SpecialTarget{
				Name: t.Name, Path: t.Path, Description: "catch-all for general notes",
			})
		}
		if t := m.cfg.IdeasTarget; t != nil {
			targets = append(targets, workmd.SpecialTarget{
				Name: t.Name, Path: t.Path, Description: "ideas not tied to a project",
			})
		}
		_ = workmd.WriteIndex(m.cfg.ExpandedIndexPath(), projects, targets)
		return projectsLoadedMsg{projects: projects}
	})
	return tea.Batch(cmds...)
}

// --- Messages ---

type tickMsg time.Time

type todoResultMsg struct {
	result string
	err    error
}

type dumpRoutedMsg struct {
	items []llm.RouteItem
	err   error
}

type dumpReroutedMsg struct {
	item *llm.RouteItem
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
