package statusbar

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const codexCacheTTL = 10 * time.Second

// codexCached reads the newest `codex.rate_limits` event from Codex's
// SQLite log and reshapes it into a Usage. Fast path: a locally-cached
// JSON snapshot of the last extracted payload. Slow path: shell out to
// `sqlite3` (no cgo).
func codexCached() (Usage, bool) {
	cachePath := cacheFile("codex-ratelimits.json")
	if body, ok := readFreshCache(cachePath, codexCacheTTL); ok {
		if u, ok := parseCodexPayload(body); ok {
			return u, true
		}
	}

	payload, ok := readCodexLatest()
	if !ok {
		if body, err := os.ReadFile(cachePath); err == nil {
			if u, ok := parseCodexPayload(body); ok {
				return u, true
			}
		}
		return Usage{}, false
	}
	_ = writeCache(cachePath, payload)
	return parseCodexPayload(payload)
}

func codexLogPath() string {
	// Honor CODEX_HOME if set (matches Codex CLI's own lookup).
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".codex")
	}
	return filepath.Join(dir, "logs_2.sqlite")
}

// readCodexLatest invokes `sqlite3` to fetch the newest log row whose
// body contains a codex.rate_limits payload, then extracts the embedded
// JSON object.
func readCodexLatest() ([]byte, bool) {
	db := codexLogPath()
	if _, err := os.Stat(db); err != nil {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	query := `SELECT feedback_log_body FROM logs ` +
		`WHERE feedback_log_body LIKE '%"codex.rate_limits"%' ` +
		`ORDER BY id DESC LIMIT 1`
	cmd := exec.CommandContext(ctx, "sqlite3", "-readonly", db, query)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	body := extractCodexJSON(string(out))
	if body == "" {
		return nil, false
	}
	return []byte(body), true
}

// extractCodexJSON finds the {"type":"codex.rate_limits", ...} object
// embedded in a log line and returns it as a standalone JSON string.
func extractCodexJSON(line string) string {
	needle := `{"type":"codex.rate_limits"`
	idx := strings.Index(line, needle)
	if idx < 0 {
		return ""
	}
	s := line[idx:]
	// Walk the string matching braces so we stop at the object's close.
	depth := 0
	inString := false
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			if inString {
				escaped = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
				if depth == 0 {
					return s[:i+1]
				}
			}
		}
	}
	return ""
}

func parseCodexPayload(body []byte) (Usage, bool) {
	var raw struct {
		RateLimits struct {
			Primary struct {
				UsedPercent float64 `json:"used_percent"`
				ResetAt     int64   `json:"reset_at"`
			} `json:"primary"`
			Secondary struct {
				UsedPercent float64 `json:"used_percent"`
				ResetAt     int64   `json:"reset_at"`
			} `json:"secondary"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Usage{}, false
	}
	u := Usage{Source: "codex"}
	u.FiveHour.Available = true
	u.FiveHour.PctUsed = int(raw.RateLimits.Primary.UsedPercent + 0.5)
	if raw.RateLimits.Primary.ResetAt > 0 {
		u.FiveHour.ResetAt = time.Unix(raw.RateLimits.Primary.ResetAt, 0)
	}
	u.SevenDay.Available = true
	u.SevenDay.PctUsed = int(raw.RateLimits.Secondary.UsedPercent + 0.5)
	if raw.RateLimits.Secondary.ResetAt > 0 {
		u.SevenDay.ResetAt = time.Unix(raw.RateLimits.Secondary.ResetAt, 0)
	}
	return u, true
}
