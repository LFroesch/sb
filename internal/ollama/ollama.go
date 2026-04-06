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
const CleanupPrompt = `You are cleaning up a WORK.md file. Preserve ALL content — do not drop any tasks, notes, plans, or prose. Losing ANY content is a complete failure.

Standard sections (normalize tasks INTO these):

## Current Tasks

- important/blockers/big features currently working out/bugs

## Backlog / Feature Ideas

- solid ideas that aren't blockers/important

## Inbox

- plain items

Rules: [ YOU CANNOT BREAK THESE ]
- Move free-text TASK lines into the correct section (Current Tasks / Backlog / Inbox)
- Keep Current Tasks short; overflow/nonimportant tasks go to Backlog
- Inbox is ONLY for items that were already in Inbox or completely unclassified loose text. Do NOT move Backlog items to Inbox.
- Remove table formatting/priority emojis+status — convert to plain lists with descriptions
- When converting table rows to list items, use " — " (em dash) to separate the task name from its description. Example: "- Task name — description here"
- Bare text lines floating outside any list are loose tasks/notes — add "- " prefix and sort them into the appropriate section (Current Tasks, Backlog, or Inbox)
- Output ONLY the cleaned markdown, no commentary, no code fences
- Do NOT duplicate items. Each item from the input should appear exactly ONCE in the output.
- EVERY item in the input MUST appear in the output. Even short/vague items — keep them as-is
- Any section that is NOT "Current Tasks", "Backlog / Feature Ideas", or "Inbox" SHOULD BE UNCHANGED — same heading, same body, same position relative to other sections. Examples: "## Current Phase", "## --schema Plan", "## Design Notes", "## API Spec". Do NOT drop, summarize, or merge them.
- If you aren't sure whether something is a task or a note, KEEP IT as-is in its original location`

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

If the text doesn't clearly belong to any specific project, route to "SECOND_BRAIN" with section "inbox". main means the main SECOND BRAIN WORK.md, not the project "main" which doesnt exist, drop the "- main" or "- project" if the intent is just to communicate to you where it should go

Respond with ONLY valid JSON: {"project": "name", "section": "inbox"}

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

Valid sections: inbox, backlog, current_tasks

Respond with ONLY a valid JSON array. Each element: {"text": "the extracted item", "project": "name", "section": "inbox"}

Example input: "need to fix the login bug in gather, also had an idea for a new tui app called radar, and sb needs better diff views"
Example output: [{"text":"fix the login bug","project":"gather","section":"current_tasks"},{"text":"new tui app idea: radar","project":"IDEAS","section":"inbox"},{"text":"sb needs better diff views","project":"sb","section":"inbox"}]

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

	routeLog(text, chatResp.Message.Content, items)
	return items, nil
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
Special: "IDEAS" for ideas not tied to a project, "SECOND_BRAIN" for general notes.
Valid sections: inbox, backlog, current_tasks

Respond with ONLY valid JSON: {"text": "the item", "project": "name", "section": "inbox"}

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

	return &item, nil
}
