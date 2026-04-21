package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/LFroesch/sb/internal/config"
)

type Client struct {
	host  string
	model string
}

func New() *Client {
	cfg := config.Load()
	return &Client{host: cfg.OllamaHost, model: cfg.Model}
}

// chat sends a single-message chat request to ollama and returns the raw
// response content. Shared plumbing for every prompt function in this package.
func (c *Client) chat(ctx context.Context, prompt string, timeout time.Duration, opts map[string]any) (string, error) {
	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if opts != nil {
		body["options"] = opts
	}
	data, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("ollama decode: %w", err)
	}
	return chatResp.Message.Content, nil
}

// RouteItem represents a single routed item from a multi-item brain dump.
type RouteItem struct {
	Text    string `json:"text"`
	Project string `json:"project"`
	Section string `json:"section"`
}

// ProjectDesc gives the router a name + a short description for each project,
// so the model isn't trying to disambiguate between bare slugs alone.
type ProjectDesc struct {
	Name        string
	Description string
}

// SpecialTarget mirrors config.Target for routing-prompt purposes. nil means
// "this target isn't configured — omit it from the prompt entirely."
type SpecialTarget struct {
	Name string
	// Description is a short reminder of what the target is *for* — e.g.
	// "catch-all for general notes". Used inside the prompt only.
	Description string
}

// renderProjectList builds the "Available projects" prompt block.
func renderProjectList(projects []ProjectDesc) string {
	var b strings.Builder
	for _, p := range projects {
		if p.Description == "" {
			fmt.Fprintf(&b, "- %s\n", p.Name)
		} else {
			fmt.Fprintf(&b, "- %s — %s\n", p.Name, p.Description)
		}
	}
	return b.String()
}

// renderSpecialTargets builds the "Special targets" prompt block, including
// CLARIFY. Catchall/ideas lines only appear when their target is non-nil.
func renderSpecialTargets(catchall, ideas *SpecialTarget) string {
	var b strings.Builder
	b.WriteString("Special targets:\n")
	if catchall != nil {
		fmt.Fprintf(&b, "- %q — %s\n", catchall.Name, catchall.Description)
	}
	if ideas != nil {
		fmt.Fprintf(&b, "- %q — %s\n", ideas.Name, ideas.Description)
	}
	b.WriteString("- \"CLARIFY\" — ONLY when you genuinely cannot tell which project an item belongs to. Most items should be routable.\n")
	return b.String()
}

// defaultUnsureLine returns the fallback instruction for ambiguous items, using
// the catchall name if configured, else falling back to CLARIFY.
func defaultUnsureLine(catchall *SpecialTarget) string {
	if catchall != nil {
		return fmt.Sprintf("If you're unsure where an item goes, default to project %q section \"current_tasks\".", catchall.Name)
	}
	return "If you're unsure between two projects, prefer \"CLARIFY\" over guessing."
}

// CleanupPrompt is the system prompt for WORK.md normalization.
const CleanupPrompt = `You are a WORK.md file organizer. Your only job is to reorganize existing content into canonical sections. You must not add, remove, rewrite, or invent anything.

ABSOLUTE RULES — violating any of these is total failure:
- DO NOT add any text that is not in the input. No "None noted", no summaries, nothing invented.
- DO NOT drop any item. Every bullet, note, and line from the input must appear in the output.
- DO NOT rewrite or rephrase task text. Copy each bullet word-for-word, character-for-character.
- Each item appears EXACTLY ONCE. Never repeat an item in multiple sections.

Structure rules:
1. First line: keep the "# WORK - slug" title exactly as-is.
2. Canonical sections in this order (only create a section if items belong there):
   ## Current Phase
   ## Current Tasks
   ## Bugs + Blockers
   ## Updates + Features
   ## Backlog
   ## Unsorted
3. Merge variant headers into canonical ones:
   Backlog / Feature Ideas / Ideas / Wishlist → ## Backlog
   Bugs / Blockers / Issues / Known Issues → ## Bugs + Blockers
   Updates / Features / Enhancements / Planned → ## Updates + Features
   Inbox / Unsorted / Misc / Notes / Dump / TODO → ## Unsorted
   Current / Active / In Progress / Sprint → ## Current Tasks
   Phase / Status / Current Phase → ## Current Phase
   Truly non-canonical headers (Design Notes, API Spec, etc.) — keep as-is, place after canonical sections.
4. Blank line after every ## heading.
5. Convert table rows to plain bullets: "- task text" (drop priority/status columns, keep the task description verbatim).
6. Output ONLY the cleaned markdown. No commentary, no code fences.`

// Cleanup sends a WORK.md file to ollama for normalization and returns the cleaned content.
// If feedback is non-empty it's appended so the model can course-correct a prior attempt.
func (c *Client) Cleanup(ctx context.Context, content, feedback string) (string, error) {
	prompt := CleanupPrompt + "\n\nHere is the WORK.md to clean up:\n\n" + content
	if feedback != "" {
		prompt += "\n\nUser feedback on the previous cleanup attempt: " + feedback +
			"\nPlease address this feedback in your cleanup."
	}

	// Deterministic sampling — cleanup is a structural normalize, not creative.
	// Same input should give the same output.
	raw, err := c.chat(ctx, prompt, 120*time.Second, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return "", err
	}
	slog.Info("ollama cleanup", "prompt", prompt, "response", raw)

	project := projectNameFromContent(content)
	cleaned := strings.TrimSpace(raw)
	// Strip accidental code fences
	cleaned = strings.TrimPrefix(cleaned, "```markdown")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = normalizeContent(strings.TrimSpace(cleaned))
	cleaned = stripProjectTagsFromBullets(cleaned, project)
	cleaned = reconcileMissingBullets(content, cleaned)
	cleaned = ensureHeaderNewlines(cleaned)
	return cleaned + "\n", nil
}

// RouteMulti splits a brain dump into discrete items and routes each one.
// Items with project "CLARIFY" need user clarification.
func (c *Client) RouteMulti(ctx context.Context, text string, projects []ProjectDesc, catchall, ideas *SpecialTarget) ([]RouteItem, error) {
	prompt := fmt.Sprintf(`You are a brain dump router. The user pasted a messy brain dump that may contain MULTIPLE ideas, tasks, or notes for DIFFERENT projects.

Your job:
1. Split the text into discrete items (one idea/task/note each)
2. Route each item to the best matching project and section. Use the project descriptions to disambiguate — slug names alone may collide.

Available projects:
%s
%s
Section guide:
- current_tasks: active work (default when unsure between current_tasks and bugs_blockers)
- bugs_blockers: something broken, failing, crashing, or actively blocking progress — urgent fix needed
- updates_features: planned improvements, enhancements, new features
- backlog: future ideas, someday/maybe items, low priority
- unsorted: genuinely unclear

%s

Respond with ONLY a valid JSON array. Each element: {"text": "the extracted item", "project": "name", "section": "current_tasks"}

Brain dump:
%s`, renderProjectList(projects), renderSpecialTargets(catchall, ideas), defaultUnsureLine(catchall), text)

	// Deterministic sampling — routing is a classification task, same dump should
	// route the same way every time.
	raw, err := c.chat(ctx, prompt, 3*time.Minute, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return nil, err
	}

	// Extract JSON array from response
	content := strings.TrimSpace(raw)
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in response: %s", content)
	}
	content = content[start : end+1]

	var items []RouteItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return nil, fmt.Errorf("parse routes: %w (raw: %s)", err, raw)
	}

	for i := range items {
		items[i].Project = strings.TrimSpace(strings.TrimPrefix(items[i].Project, "#"))
		items[i].Text = stripProjectTag(items[i].Text, items[i].Project)
	}

	routeLog(text, raw, items)
	return items, nil
}

// NextTodo asks ollama to suggest what to work on next given a WORK.md.
func (c *Client) NextTodo(ctx context.Context, content string) (string, error) {
	prompt := `You are a helpful assistant reviewing a WORK.md task file. Give a short, direct answer: what are the 2-3 most important things to work on right now? Be specific and actionable. No fluff.

Priority order: Bugs + Blockers first (urgent), then Current Tasks, then Unsorted. If there are bugs/blockers, always lead with those.

WORK.md:
` + content

	raw, err := c.chat(ctx, prompt, 60*time.Second, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(raw), nil
}

// DailyPlan asks ollama to group tasks from multiple projects by theme and suggest a day plan.
func (c *Client) DailyPlan(ctx context.Context, taskSummary string) (string, error) {
	prompt := `You are a daily planning assistant. Given current tasks from multiple projects, group similar tasks by context/theme and produce a focused plan for today as clean markdown.

Rules:
- Create 2-4 context groups (e.g. "Bugs + Blockers", "Tooling", "Active Dev")
- Pick 1-3 specific tasks per group that would move the needle today
- Bugs + Blockers are URGENT — if any exist, surface them first under a "Bugs + Blockers" group regardless of project
- Lead with highest-impact group (bugs/blockers group always first if present)
- Copy task text verbatim from the input. Do not rephrase or invent tasks.
- Each task is one bullet with the project name in parens at the end

Output must be valid markdown using ONLY this structure:
# Daily Plan

## Group Name
- task description (project-name)
- another task (project-name)

## Next Group Name
- task description (project-name)

No commentary, no preamble, no trailing notes — just the markdown plan.

Current tasks:
` + taskSummary

	raw, err := c.chat(ctx, prompt, 90*time.Second, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return "", err
	}
	slog.Info("ollama daily plan", "prompt", prompt, "response", raw)
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```markdown")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned), nil
}

// reconcileMissingBullets checks that every bullet from the original content appears in the
// cleaned output. Any missing bullets are appended to ## Unsorted as a safety net.
func reconcileMissingBullets(original, cleaned string) string {
	extractBullets := func(s string) []string {
		var bullets []string
		for _, line := range strings.Split(s, "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "- ") {
				bullets = append(bullets, strings.ToLower(strings.TrimSpace(t[2:])))
			}
		}
		return bullets
	}

	outBullets := extractBullets(cleaned)

	outSet := make(map[string]bool, len(outBullets))
	for _, b := range outBullets {
		outSet[b] = true
	}

	// Collect original lines for missing bullets (preserve exact text)
	var missing []string
	for _, line := range strings.Split(original, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") {
			key := strings.ToLower(strings.TrimSpace(t[2:]))
			if !outSet[key] {
				missing = append(missing, t)
				outSet[key] = true // don't add twice
			}
		}
	}

	if len(missing) == 0 {
		return cleaned
	}

	// Append to existing ## Unsorted or add the section
	unsortedHeader := "## Unsorted"
	if strings.Contains(cleaned, unsortedHeader) {
		// Insert after the ## Unsorted header line
		lines := strings.Split(cleaned, "\n")
		out := make([]string, 0, len(lines)+len(missing)+1)
		inserted := false
		for i, line := range lines {
			out = append(out, line)
			if !inserted && strings.TrimSpace(line) == unsortedHeader {
				// skip blank line after header if present, then insert
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
					out = append(out, lines[i+1])
					i++ // will be incremented by loop but we already appended
					_ = i
				}
				for _, m := range missing {
					out = append(out, m)
				}
				inserted = true
			}
		}
		return strings.Join(out, "\n")
	}

	// No ## Unsorted section — append it
	result := strings.TrimRight(cleaned, "\n") + "\n\n## Unsorted\n\n"
	for _, m := range missing {
		result += m + "\n"
	}
	return result
}

// ensureHeaderNewlines guarantees a blank line after every ## heading.
func ensureHeaderNewlines(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+4)
	for i, line := range lines {
		out = append(out, line)
		if strings.HasPrefix(strings.TrimSpace(line), "##") {
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				out = append(out, "")
			}
		}
	}
	return strings.Join(out, "\n")
}

// normalizeContent converts table rows and emoji-priority lists to plain bullets.
// Strips priority emoji (🔴🟡🟢🔵⚪🟠) and status columns, leaving task + optional note.
func normalizeContent(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	priorityEmoji := []string{"🔴", "🟡", "🟢", "🔵", "⚪", "🟠"}
	priorityWords := []string{"high", "med", "medium", "low", "critical"}

	stripPriority := func(s string) string {
		s = strings.TrimSpace(s)
		for _, e := range priorityEmoji {
			s = strings.TrimPrefix(s, e)
			s = strings.TrimSpace(s)
		}
		// Strip trailing priority word that got left (e.g. "High" after emoji)
		lower := strings.ToLower(s)
		for _, w := range priorityWords {
			if lower == w || strings.HasPrefix(lower, w+" ") {
				s = strings.TrimSpace(s[len(w):])
				break
			}
		}
		return strings.TrimSpace(s)
	}

	isStatusWord := func(s string) bool {
		lower := strings.ToLower(strings.TrimSpace(s))
		return lower == "done" || lower == "todo" || lower == "in progress" ||
			lower == "blocked" || lower == "wip" || lower == "pending" ||
			lower == "high" || lower == "med" || lower == "medium" || lower == "low"
	}

	isPriorityCol := func(s string) bool {
		s = strings.TrimSpace(s)
		for _, e := range priorityEmoji {
			if strings.HasPrefix(s, e) {
				return true
			}
		}
		return false
	}

	tableColToPlain := func(trimmed string) (string, bool) {
		if !strings.HasPrefix(trimmed, "|") {
			return "", false
		}
		cols := strings.Split(trimmed, "|")
		var parts []string
		for _, c := range cols {
			if s := strings.TrimSpace(c); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			return "", true // skip
		}
		// Skip header rows
		for _, p := range parts {
			lower := strings.ToLower(p)
			if lower == "task" || lower == "status" || lower == "priority" ||
				lower == "item" || lower == "#" || lower == "notes" {
				return "", true
			}
		}
		// Collect meaningful columns: skip priority/status cols
		var meaningful []string
		for _, p := range parts {
			p = stripPriority(p)
			if p == "" || isStatusWord(p) || isPriorityCol(p) {
				continue
			}
			meaningful = append(meaningful, p)
		}
		if len(meaningful) == 0 {
			return "", true
		}
		if len(meaningful) == 1 {
			return "- " + meaningful[0], true
		}
		// task — note
		return "- " + meaningful[0] + " — " + strings.Join(meaningful[1:], ", "), true
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip table separator rows
		if strings.HasPrefix(trimmed, "|--") || strings.HasPrefix(trimmed, "| --") ||
			strings.HasPrefix(trimmed, "|:-") || strings.HasPrefix(trimmed, "| :-") {
			continue
		}

		// Convert table rows to plain bullets
		if strings.HasPrefix(trimmed, "|") {
			if result, ok := tableColToPlain(trimmed); ok {
				if result != "" {
					out = append(out, result)
				}
			} else {
				out = append(out, line)
			}
			continue
		}

		// Handle bullet items (including indented): strip emoji priority and pipe residue
		isBullet := strings.HasPrefix(trimmed, "- ")
		if isBullet {
			item := trimmed[2:]
			// Strip priority emoji + word prefix
			item = stripPriority(item)
			// If item still contains pipes (model converted table row to bullet but kept pipes)
			if strings.Contains(item, "|") {
				cols := strings.Split(item, "|")
				var parts []string
				for _, c := range cols {
					c = strings.TrimSpace(c)
					if c == "" || isStatusWord(c) || isPriorityCol(c) {
						continue
					}
					parts = append(parts, c)
				}
				if len(parts) == 0 {
					continue
				} else if len(parts) == 1 {
					item = parts[0]
				} else {
					item = parts[0] + " — " + strings.Join(parts[1:], ", ")
				}
			}
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, "- "+item)
			continue
		}

		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripProjectTag removes patterns like "projectName - text" or "text - projectName"
// that the model sometimes adds to tell where the item should be filed.
func stripProjectTag(text, project string) string {
	text = strings.TrimSpace(text)
	if project == "" {
		return text
	}
	lc := strings.ToLower(text)
	lp := strings.ToLower(project)

	if strings.HasPrefix(lc, lp+" - ") {
		return strings.TrimSpace(text[len(lp)+3:])
	}
	if strings.HasSuffix(lc, " - "+lp) {
		return strings.TrimSpace(text[:len(text)-len(lp)-3])
	}
	return text
}

// projectNameFromContent extracts the slug from a "# WORK - slug" title line.
func projectNameFromContent(content string) string {
	for _, line := range strings.SplitN(content, "\n", 5) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# WORK - ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# WORK - "))
		}
		if strings.HasPrefix(line, "# WORK") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# WORK"))
		}
	}
	return ""
}

// stripProjectTagsFromBullets applies stripProjectTag to every bullet line in content.
func stripProjectTagsFromBullets(content, project string) string {
	if project == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			cleaned := stripProjectTag(trimmed[2:], project)
			lines[i] = "- " + cleaned
		}
	}
	return strings.Join(lines, "\n")
}

// routeLog records the raw routing response plus parsed items for debugging.
func routeLog(input, raw string, items []RouteItem) {
	slog.Info("ollama route", "input", input, "raw", raw, "items", items)
}

// RerouteSingle re-routes a single item with user-provided clarification context.
func (c *Client) RerouteSingle(ctx context.Context, text, clarification string, projects []ProjectDesc, catchall, ideas *SpecialTarget) (*RouteItem, error) {
	prompt := fmt.Sprintf(`Route this item to a project. The user clarified: %q

Available projects:
%s
%s
Sections: current_tasks (active work, default), bugs_blockers (broken/actively blocking), updates_features (planned improvements), backlog (future/low-prio), unsorted (unclear)

Respond with ONLY valid JSON: {"text": "the item", "project": "name", "section": "current_tasks"}

Item: %s`, clarification, renderProjectList(projects), renderSpecialTargets(catchall, ideas), text)

	raw, err := c.chat(ctx, prompt, 30*time.Second, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(raw)
	if idx := strings.Index(content, "{"); idx >= 0 {
		end := strings.LastIndex(content, "}")
		if end > idx {
			content = content[idx : end+1]
		}
	}

	var item RouteItem
	if err := json.Unmarshal([]byte(content), &item); err != nil {
		return nil, fmt.Errorf("parse reroute: %w (raw: %s)", err, raw)
	}

	item.Project = strings.TrimSpace(strings.TrimPrefix(item.Project, "#"))
	item.Text = stripProjectTag(item.Text, item.Project)
	return &item, nil
}
