// Package statusbar exposes provider usage snapshots (5h/7d rolling
// limits) for rendering inside the sb TUI. The global tmux statusline
// integration lives in shell scripts outside this package; this file
// only powers the in-process dashboard view.
package statusbar

import (
	"sync"
	"time"
)

// Usage is the common shape produced by both providers.
type Usage struct {
	Source   string // "claude" | "codex"
	FiveHour Window
	SevenDay Window
	Extra    *Extra
}

type Window struct {
	PctUsed   int
	ResetAt   time.Time
	Available bool
}

type Extra struct {
	PctUsed      int
	UsedCredits  float64
	MonthlyLimit float64
	Enabled      bool
}

// FetchClaude returns the current Claude usage snapshot, or ok=false
// when credentials are missing or the endpoint is unreachable. A small
// in-memory TTL on top of the on-disk cache keeps TUI renders cheap.
func FetchClaude() (Usage, bool) {
	memoMu.Lock()
	defer memoMu.Unlock()
	if time.Since(claudeMemoAt) < memoTTL && claudeMemoSet {
		return claudeMemo, claudeMemoOK
	}
	claudeMemo, claudeMemoOK = claudeCached()
	claudeMemoAt = time.Now()
	claudeMemoSet = true
	return claudeMemo, claudeMemoOK
}

// FetchCodex returns the current Codex usage snapshot from the local
// sqlite log, or ok=false if none is available.
func FetchCodex() (Usage, bool) {
	memoMu.Lock()
	defer memoMu.Unlock()
	if time.Since(codexMemoAt) < memoTTL && codexMemoSet {
		return codexMemo, codexMemoOK
	}
	codexMemo, codexMemoOK = codexCached()
	codexMemoAt = time.Now()
	codexMemoSet = true
	return codexMemo, codexMemoOK
}

const memoTTL = 5 * time.Second

var (
	memoMu        sync.Mutex
	claudeMemo    Usage
	claudeMemoOK  bool
	claudeMemoSet bool
	claudeMemoAt  time.Time
	codexMemo     Usage
	codexMemoOK   bool
	codexMemoSet  bool
	codexMemoAt   time.Time
)
