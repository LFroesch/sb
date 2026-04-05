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

// CleanupPrompt is the system prompt for WORK.md normalization.
const CleanupPrompt = `You are cleaning up a WORK.md file. Preserve ALL content — do not drop any tasks or notes. Losing any notes/tasks results in a complete failure.

Standard format:

## Current Tasks

- important/blockers/big features currently working out/bugs

## Backlog / Feature Ideas

- solid ideas that aren't blockers/important

## Inbox

- plain items

Rules:
- Move free-text lines/tasks into the correct section (Current Tasks / Backlog / Inbox)
- Keep Current Tasks short; overflow/NONIMPORTANT tasks go to Backlog
- Remove any table formatting/priority bubbles, i just want lists with descriptions
- Output ONLY the cleaned markdown, no commentary, no code fences
- Condense duplicate/similar items but do not delete words/descriptors
- If you aren't sure, do not delete it. DO NOT DELETE TASKS, NEVER DELETE ANY TASKS. NO CONTEXT OR CONTENT SHOULD BE LOST`

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

	cleaned := strings.TrimSpace(chatResp.Message.Content)
	// Strip accidental code fences
	cleaned = strings.TrimPrefix(cleaned, "```markdown")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned) + "\n", nil
}

// Route classifies a brain dump and returns the target project + section.
// TODO: MAKE IT WORK FOR MANY VARIOUS IDEAS IN ONE BRAIN DUMP, FOR MULTIPLE DIFFERENT PROJECTS/IDEAS ETC
func (c *Client) Route(ctx context.Context, text string, projectNames []string) (*RouteResult, error) {
	prompt := fmt.Sprintf(`You are a brain dump router. Given a thought/idea/task, decide which project it belongs to and which section (inbox, backlog, or current_tasks).

Available projects: %s

If the text doesn't clearly belong to any specific project, route to "SECOND_BRAIN" with section "inbox".

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
