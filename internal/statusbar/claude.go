package statusbar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// tmux runs `sb tmux-status` in a fresh process on each refresh, so the
	// in-memory memoization in statusbar.go does not help there. Keep a longer
	// on-disk TTL so the status line stays responsive without hammering Claude's
	// oauth/usage endpoint into rate limits.
	claudeCacheTTL = 5 * time.Minute
	claudeUsageURL = "https://api.anthropic.com/api/oauth/usage"
	claudeUA       = "claude-code/2.1.34"
	claudeBeta     = "oauth-2025-04-20"
)

// claudeCached returns the Claude block, hitting cache first then
// falling back to the usage endpoint. (Usage{}, false) on any failure
// so the caller can silently skip rendering.
func claudeCached() (Usage, bool) {
	cachePath := cacheFile("claude-usage.json")
	if body, ok := readFreshCache(cachePath, claudeCacheTTL); ok {
		if u, ok := parseClaudeUsage(body); ok {
			return u, true
		}
	}

	token := readClaudeToken()
	if token == "" {
		// No creds — maybe previous cache is still usable (stale).
		if body, err := os.ReadFile(cachePath); err == nil {
			if u, ok := parseClaudeUsage(body); ok {
				return u, true
			}
		}
		return Usage{}, false
	}

	body, err := fetchClaudeUsage(token)
	if err != nil || len(body) == 0 {
		// Network failed — use stale cache if available.
		if stale, rerr := os.ReadFile(cachePath); rerr == nil {
			if u, ok := parseClaudeUsage(stale); ok {
				return u, true
			}
		}
		return Usage{}, false
	}
	_ = writeCache(cachePath, body)

	return parseClaudeUsage(body)
}

func readClaudeToken() string {
	dir := claudeConfigDir()
	// Direct credentials file.
	credPath := filepath.Join(dir, ".credentials.json")
	if b, err := os.ReadFile(credPath); err == nil {
		var env struct {
			ClaudeAiOauth struct {
				AccessToken string `json:"accessToken"`
			} `json:"claudeAiOauth"`
		}
		if jerr := json.Unmarshal(b, &env); jerr == nil && env.ClaudeAiOauth.AccessToken != "" {
			return env.ClaudeAiOauth.AccessToken
		}
	}
	if tok := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); tok != "" {
		return tok
	}
	return ""
}

func claudeConfigDir() string {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func fetchClaudeUsage(token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, claudeUsageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", claudeBeta)
	req.Header.Set("User-Agent", claudeUA)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude usage: status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16*1024))
}

// parseClaudeUsage takes the JSON body returned by oauth/usage (or a
// cached copy of it) and extracts the bits we render.
func parseClaudeUsage(body []byte) (Usage, bool) {
	var raw struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
		ExtraUsage struct {
			IsEnabled    bool    `json:"is_enabled"`
			Utilization  float64 `json:"utilization"`
			UsedCredits  float64 `json:"used_credits"`
			MonthlyLimit float64 `json:"monthly_limit"`
		} `json:"extra_usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Usage{}, false
	}
	u := Usage{Source: "claude"}
	u.FiveHour.Available = true
	u.FiveHour.PctUsed = int(raw.FiveHour.Utilization + 0.5)
	u.FiveHour.ResetAt = parseISOTime(raw.FiveHour.ResetsAt)
	u.SevenDay.Available = true
	u.SevenDay.PctUsed = int(raw.SevenDay.Utilization + 0.5)
	u.SevenDay.ResetAt = parseISOTime(raw.SevenDay.ResetsAt)
	if raw.ExtraUsage.IsEnabled {
		u.Extra = &Extra{
			Enabled:      true,
			PctUsed:      int(raw.ExtraUsage.Utilization + 0.5),
			UsedCredits:  raw.ExtraUsage.UsedCredits / 100,
			MonthlyLimit: raw.ExtraUsage.MonthlyLimit / 100,
		}
	}
	return u, true
}

func parseISOTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// oauth/usage returns RFC3339 with fractional seconds and Z.
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
