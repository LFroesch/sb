package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	host  string
	model string
}

func New() *Client {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	} else if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	model := os.Getenv("SB_MODEL")
	if model == "" {
		model = "qwen2.5:7b"
	}
	return &Client{host: host, model: model}
}

type RouteResult struct {
	Project string `json:"project"`
	Section string `json:"section"`
}

// RouteItem represents a single routed item from a multi-item brain dump.
type RouteItem struct {
	Text    string `json:"text"`
	Project string `json:"project"`
	Section string `json:"section"`
}

// CleanupPrompt is the system prompt for WORK.md normalization.
const CleanupPrompt = `Clean up this WORK.md file. Rules:

1. NEVER DROP CONTENT. Every task, note, bullet, and line must appear exactly once in the output. Losing even one item is failure. If unsure where something goes, put it in ## Unsorted.
2. Keep the "# WORK - slug" title as the very first line.
3. Canonical sections IN THIS ORDER (create only if items belong there):
   ## Current Phase    (one-liner: what the project is doing right now)
   ## Current Tasks    (active work, in-progress items)
   ## Bugs + Blockers  (bugs, blockers, broken things)
   ## Updates + Features (enhancements, improvements, planned features)
   ## Backlog          (ideas, low-priority, not urgent)
   ## Unsorted         (anything that doesn't fit above)
4. MERGE old/variant headers into canonical ones:
   Backlog, Feature Ideas, Ideas, Wishlist, Nice to Have → ## Backlog
   Bugs, Blockers, Issues, Known Issues, Broken → ## Bugs + Blockers
   Updates, Features, Enhancements, Improvements, Planned → ## Updates + Features
   Inbox, Unsorted, Misc, Notes, Dump, TODO → ## Unsorted
   Current, Active, In Progress, Doing, Sprint → ## Current Tasks
   Phase, Status, Current Phase → ## Current Phase
   Any section that doesn't map to the above is TRULY non-canonical (## Design Notes, ## API Spec, etc.) — keep those as-is after the canonical sections.
5. Always leave a blank line after every ## heading.
6. Convert tables or emoji-status lists to plain "- item" bullet lists.
7. Deduplicate exact duplicates only. If two items are similar but not identical, keep both.
8. Output ONLY the cleaned markdown. No commentary, no code fences.`

// cleanupLog writes a cleanup request/response pair to /tmp/sb-cleanup.log for tuning.
func cleanupLog(prompt, response string) {
	f, err := os.OpenFile("/tmp/sb-cleanup.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	sep := strings.Repeat("=", 80)
	fmt.Fprintf(f, "%s\n[%s] CLEANUP REQUEST\n%s\n%s\n", sep, time.Now().Format(time.RFC3339), sep, prompt)
	fmt.Fprintf(f, "%s\n[%s] CLEANUP RESPONSE\n%s\n%s\n\n", sep, time.Now().Format(time.RFC3339), sep, response)
}

// Cleanup sends a WORK.md file to ollama for normalization and returns the cleaned content.
func (c *Client) Cleanup(ctx context.Context, content string) (string, error) {
	prompt := CleanupPrompt + "\n\nHere is the WORK.md to clean up:\n\n" + content

	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
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

	cleanupLog(prompt, chatResp.Message.Content)

	cleaned := strings.TrimSpace(chatResp.Message.Content)
	// Strip accidental code fences
	cleaned = strings.TrimPrefix(cleaned, "```markdown")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned) + "\n", nil
}

// Route classifies a brain dump and returns the target project + section.
func (c *Client) Route(ctx context.Context, text string, projectNames []string) (*RouteResult, error) {
	prompt := fmt.Sprintf(`You are a brain dump router. Given a thought/idea/task, decide which project it belongs to and which section (inbox, backlog, or current_tasks).

Available projects: %s

If the text doesn't clearly belong to any specific project, route to "SECOND_BRAIN" with section "current_tasks". "main" means the main SECOND_BRAIN WORK.md, not a project called "main".

Valid sections: current_tasks, bugs_blockers, updates_features, backlog, unsorted
Respond with ONLY valid JSON: {"project": "name", "section": "current_tasks"}

Brain dump: %s`, strings.Join(projectNames, ", "), text)

	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}

	// Extract JSON from response (may have markdown wrapping)
	content := chatResp.Message.Content
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "{"); idx >= 0 {
		end := strings.LastIndex(content, "}")
		if end > idx {
			content = content[idx : end+1]
		}
	}

	var result RouteResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse route: %w (raw: %s)", err, chatResp.Message.Content)
	}

	return &result, nil
}

// RouteMulti splits a brain dump into discrete items and routes each one.
// Items with project "CLARIFY" need user clarification.
func (c *Client) RouteMulti(ctx context.Context, text string, projectNames []string) ([]RouteItem, error) {
	prompt := fmt.Sprintf(`You are a brain dump router. The user pasted a messy brain dump that may contain MULTIPLE ideas, tasks, or notes for DIFFERENT projects.

Your job:
1. Split the text into discrete items (one idea/task/note each)
2. Route each item to the best matching project and section

Available projects: %s
Special targets:
- "IDEAS" — for ideas that don't belong to any current project
- "SECOND_BRAIN" — catch-all for general notes
- "CLARIFY" — ONLY use this when you genuinely cannot determine which project an item belongs to. Most items should be routable.

Valid sections: current_tasks, bugs_blockers, updates_features, backlog, unsorted
If you're unsure where an item goes, default to project "SECOND_BRAIN" section "current_tasks".

Respond with ONLY a valid JSON array. Each element: {"text": "the extracted item", "project": "name", "section": "current_tasks"}

Example input: "need to fix the login bug in gather, also had an idea for a new tui app called radar, and sb needs better diff views"
Example output: [{"text":"fix the login bug","project":"gather","section":"bugs_blockers"},{"text":"new tui app idea: radar","project":"IDEAS","section":"backlog"},{"text":"sb needs better diff views","project":"sb","section":"updates_features"}]

Brain dump:
%s`, strings.Join(projectNames, ", "), text)

	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}

	// Extract JSON array from response
	content := strings.TrimSpace(chatResp.Message.Content)
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in response: %s", content)
	}
	content = content[start : end+1]

	var items []RouteItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		return nil, fmt.Errorf("parse routes: %w (raw: %s)", err, chatResp.Message.Content)
	}

	for i := range items {
		items[i].Project = strings.TrimSpace(strings.TrimPrefix(items[i].Project, "#"))
	}

	routeLog(text, chatResp.Message.Content, items)
	return items, nil
}

// NextTodo asks ollama to suggest what to work on next given a WORK.md.
func (c *Client) NextTodo(ctx context.Context, content string) (string, error) {
	prompt := `You are a helpful assistant reviewing a WORK.md task file. Based on the Current Tasks and Inbox sections, give a short, direct answer: what are the 2-3 most important things to work on right now? Be specific and actionable. No fluff.

WORK.md:
` + content

	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
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
	return strings.TrimSpace(chatResp.Message.Content), nil
}

// routeLog writes a routing request/response pair to /tmp/sb-route.log for debugging.
func routeLog(input, raw string, items []RouteItem) {
	f, err := os.OpenFile("/tmp/sb-route.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	sep := strings.Repeat("=", 60)
	fmt.Fprintf(f, "%s\n[%s] INPUT\n%s\n%s\n", sep, time.Now().Format(time.RFC3339), sep, input)
	fmt.Fprintf(f, "RAW RESPONSE:\n%s\n", raw)
	fmt.Fprintf(f, "PARSED (%d items):\n", len(items))
	for i, it := range items {
		fmt.Fprintf(f, "  [%d] %q → %s/%s\n", i, it.Text, it.Project, it.Section)
	}
	fmt.Fprintln(f)
}

// RerouteSingle re-routes a single item with user-provided clarification context.
func (c *Client) RerouteSingle(ctx context.Context, text, clarification string, projectNames []string) (*RouteItem, error) {
	prompt := fmt.Sprintf(`Route this item to a project. The user clarified: "%s"

Available projects: %s
Special: "IDEAS" for ideas not tied to a project, "SECOND_BRAIN" for general notes (default to current_tasks if unsure).
Valid sections: current_tasks, bugs_blockers, updates_features, backlog, unsorted

Respond with ONLY valid JSON: {"text": "the item", "project": "name", "section": "current_tasks"}

Item: %s`, clarification, strings.Join(projectNames, ", "), text)

	body := map[string]any{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	var chatResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("ollama decode: %w", err)
	}

	content := strings.TrimSpace(chatResp.Message.Content)
	if idx := strings.Index(content, "{"); idx >= 0 {
		end := strings.LastIndex(content, "}")
		if end > idx {
			content = content[idx : end+1]
		}
	}

	var item RouteItem
	if err := json.Unmarshal([]byte(content), &item); err != nil {
		return nil, fmt.Errorf("parse reroute: %w (raw: %s)", err, chatResp.Message.Content)
	}

	item.Project = strings.TrimSpace(strings.TrimPrefix(item.Project, "#"))
	return &item, nil
}
