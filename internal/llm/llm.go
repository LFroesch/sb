package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/LFroesch/sb/internal/config"
)

type Client struct {
	provider config.ProviderConfig
}

func New(cfg *config.Config) *Client {
	if cfg == nil {
		cfg = config.Load()
	}
	return &Client{provider: cfg.ActiveProvider()}
}

func (c *Client) providerLabel() string {
	if c.provider.Type == "" {
		return "llm"
	}
	return c.provider.Type
}

// chat sends a single-message request to the active provider and returns the raw
// response content. Shared plumbing for every prompt function in this package.
func (c *Client) chat(ctx context.Context, prompt string, timeout time.Duration, opts map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch c.provider.Type {
	case "openai":
		return c.chatOpenAI(ctx, prompt, opts)
	case "anthropic":
		return c.chatAnthropic(ctx, prompt, opts)
	default:
		return c.chatOllama(ctx, prompt, opts)
	}
}

func (c *Client) chatOllama(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	body := map[string]any{
		"model":  c.provider.Model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if len(opts) > 0 {
		body["options"] = opts
	}

	var resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.provider.BaseURL+"/api/chat", nil, body, &resp); err != nil {
		return "", fmt.Errorf("ollama: %w", err)
	}
	return resp.Message.Content, nil
}

func (c *Client) chatOpenAI(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	apiKey := c.provider.ResolvedAPIKey()
	if apiKey == "" {
		return "", fmt.Errorf("openai: missing api key")
	}

	body := map[string]any{
		"model": c.provider.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	for k, v := range opts {
		body[k] = v
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	headers := map[string]string{
		"Authorization": "Bearer " + apiKey,
	}
	if err := c.doJSON(ctx, http.MethodPost, strings.TrimRight(c.provider.BaseURL, "/")+"/chat/completions", headers, body, &resp); err != nil {
		return "", fmt.Errorf("openai: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) chatAnthropic(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	apiKey := c.provider.ResolvedAPIKey()
	if apiKey == "" {
		return "", fmt.Errorf("anthropic: missing api key")
	}

	body := map[string]any{
		"model":      c.provider.Model,
		"max_tokens": 2048,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	if v, ok := opts["temperature"]; ok {
		body["temperature"] = v
	}
	if v, ok := opts["top_p"]; ok {
		body["top_p"] = v
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	headers := map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}
	if err := c.doJSON(ctx, http.MethodPost, strings.TrimRight(c.provider.BaseURL, "/")+"/v1/messages", headers, body, &resp); err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	var parts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return strings.Join(parts, "\n"), nil
}

func (c *Client) doJSON(ctx context.Context, method, url string, headers map[string]string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.provider.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, msg)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
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
	Phase       string
	Preview     []string
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
		fmt.Fprintf(&b, "- %s", truncatePromptField(p.Name, 80))
		if p.Description != "" {
			fmt.Fprintf(&b, " — %s", truncatePromptField(p.Description, 160))
		}
		if p.Phase != "" {
			fmt.Fprintf(&b, " | phase: %s", truncatePromptField(p.Phase, 120))
		}
		b.WriteString("\n")
		for _, item := range truncatePromptPreview(p.Preview, 2, 120) {
			fmt.Fprintf(&b, "  active: %s\n", item)
		}
	}
	return b.String()
}

func truncatePromptField(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return strings.TrimSpace(s[:max-1]) + "…"
}

func truncatePromptPreview(items []string, maxItems, maxLen int) []string {
	if maxItems <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, truncatePromptField(item, maxLen))
	}
	return out
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
const CleanupPrompt = `You are a WORK.md file organizer. Your job is to lightly tidy the file while preserving its structure and meaning. You must not add, remove, rewrite, or invent substantive content.

ABSOLUTE RULES — violating any of these is total failure:
- DO NOT add any text that is not in the input. No "None noted", no summaries, nothing invented.
- DO NOT drop any item. Every bullet, note, and line from the input must appear in the output.
- DO NOT rewrite or rephrase task text. Copy each bullet word-for-word, character-for-character.
- Each item appears EXACTLY ONCE. Never repeat an item in multiple sections.

Structure rules:
1. Keep the first heading exactly as-is, including its file-type prefix such as "# WORK - ..." or "# ROADMAP - ...".
2. Preserve existing headers and their order unless an item is obviously under the wrong header.
3. Keep the short description line directly below the H1 if one exists.
4. You may remove exact duplicates.
5. You may normalize malformed bullets and convert obvious task tables into plain bullets.
6. Do NOT invent canonical headers or aggressively merge/rename sections.
7. Output ONLY the cleaned markdown. No commentary, no code fences.`

// Cleanup sends a WORK.md file to the active provider for normalization and
// returns the cleaned content. If feedback is non-empty it's appended so the
// model can course-correct a prior attempt.
func (c *Client) Cleanup(ctx context.Context, content, feedback string) (string, error) {
	prompt := CleanupPrompt + "\n\nHere is the WORK.md to clean up:\n\n" + content
	if feedback != "" {
		prompt += "\n\nUser feedback on the previous cleanup attempt: " + feedback +
			"\nPlease address this feedback in your cleanup."
	}

	raw, err := c.chat(ctx, prompt, 120*time.Second, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return "", err
	}
	slog.Info("llm cleanup", "provider", c.providerLabel(), "prompt", prompt, "response", raw)

	project := projectNameFromContent(content)
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```markdown")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = normalizeContent(strings.TrimSpace(cleaned))
	cleaned = stripProjectTagsFromBullets(cleaned, project)
	cleaned = reconcileMissingBullets(content, cleaned)
	cleaned = reconcileMissingSections(content, cleaned)
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

	raw, err := c.chat(ctx, prompt, 3*time.Minute, map[string]any{
		"temperature": 0,
		"top_p":       1,
		"seed":        42,
	})
	if err != nil {
		return nil, err
	}

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

	routeLog(c.providerLabel(), text, raw, items)
	return items, nil
}

// NextTodo asks the active provider to suggest what to work on next given a WORK.md.
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

// DailyPlan asks the active provider to group tasks from multiple projects by
// theme and suggest a day plan.
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
	slog.Info("llm daily plan", "provider", c.providerLabel(), "prompt", prompt, "response", raw)
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

	var missing []string
	for _, line := range strings.Split(original, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") {
			key := strings.ToLower(strings.TrimSpace(t[2:]))
			if !outSet[key] {
				missing = append(missing, t)
				outSet[key] = true
			}
		}
	}

	if len(missing) == 0 {
		return cleaned
	}

	unsortedHeader := "## Unsorted"
	if strings.Contains(cleaned, unsortedHeader) {
		lines := strings.Split(cleaned, "\n")
		out := make([]string, 0, len(lines)+len(missing)+1)
		inserted := false
		for i, line := range lines {
			out = append(out, line)
			if !inserted && strings.TrimSpace(line) == unsortedHeader {
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
					out = append(out, lines[i+1])
					i++
					_ = i
				}
				out = append(out, missing...)
				inserted = true
			}
		}
		return strings.Join(out, "\n")
	}

	result := strings.TrimRight(cleaned, "\n") + "\n\n## Unsorted\n\n"
	for _, m := range missing {
		result += m + "\n"
	}
	return result
}

func reconcileMissingSections(original, cleaned string) string {
	canonical := map[string]bool{
		"## Current Phase":      true,
		"## Current Tasks":      true,
		"## Bugs + Blockers":    true,
		"## Updates + Features": true,
		"## Backlog":            true,
		"## Unsorted":           true,
	}

	type sectionBlock struct {
		header string
		body   []string
	}

	var blocks []sectionBlock
	var current *sectionBlock
	for _, line := range strings.Split(original, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if current != nil && !canonical[current.header] {
				blocks = append(blocks, *current)
			}
			current = &sectionBlock{header: trimmed, body: []string{line}}
			continue
		}
		if current != nil {
			current.body = append(current.body, line)
		}
	}
	if current != nil && !canonical[current.header] {
		blocks = append(blocks, *current)
	}
	if len(blocks) == 0 {
		return cleaned
	}

	result := strings.TrimRight(cleaned, "\n")
	for _, block := range blocks {
		if strings.Contains(cleaned, block.header) {
			continue
		}
		result += "\n\n" + strings.TrimRight(strings.Join(block.body, "\n"), "\n")
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
			return "", true
		}
		for _, p := range parts {
			lower := strings.ToLower(p)
			if lower == "task" || lower == "status" || lower == "priority" ||
				lower == "item" || lower == "#" || lower == "notes" {
				return "", true
			}
		}
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
		return "- " + meaningful[0] + " — " + strings.Join(meaningful[1:], ", "), true
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "|--") || strings.HasPrefix(trimmed, "| --") ||
			strings.HasPrefix(trimmed, "|:-") || strings.HasPrefix(trimmed, "| :-") {
			continue
		}

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

		isBullet := strings.HasPrefix(trimmed, "- ")
		if isBullet {
			item := trimmed[2:]
			item = stripPriority(item)
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
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if _, afterDash, ok := strings.Cut(title, " - "); ok {
				name := afterDash
				if beforePipe, _, hasPipe := strings.Cut(name, "|"); hasPipe {
					name = beforePipe
				}
				return strings.TrimSpace(name)
			}
			return strings.TrimSpace(title)
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
func routeLog(provider, input, raw string, items []RouteItem) {
	slog.Info("llm route", "provider", provider, "input", input, "raw", raw, "items", items)
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
