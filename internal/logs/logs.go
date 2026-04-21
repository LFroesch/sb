package logs

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultMaxBytes = 5 << 20
	defaultKeep     = 3
)

// Open returns a slog.Logger that writes JSON lines to the sb data dir.
func Open(tag, level string) *slog.Logger {
	path := filepath.Join(xdgDataHome(), "sb", "logs", "sb.log")
	w, err := newRotatingWriter(path, defaultMaxBytes, defaultKeep)
	if err != nil {
		handler := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: parseLevel(level)})
		return slog.New(handler).With("app", tag)
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(handler).With("app", tag)
}

type rotatingWriter struct {
	path string
	max  int64
	keep int

	mu   sync.Mutex
	f    *os.File
	size int64
}

func newRotatingWriter(path string, max int64, keep int) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rotatingWriter{path: path, max: max, keep: keep, f: f, size: info.Size()}, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.max > 0 && w.size+int64(len(p)) > w.max {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) rotate() error {
	if w.f != nil {
		if err := w.f.Close(); err != nil {
			return err
		}
	}

	for i := w.keep; i >= 1; i-- {
		src := w.path
		if i > 1 {
			src = rotatedPath(w.path, i-1)
		}
		dst := rotatedPath(w.path, i)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	w.f = f
	w.size = 0
	return nil
}

func rotatedPath(path string, n int) string {
	return path + "." + strconv.Itoa(n)
}

func xdgDataHome() string {
	if v := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "share")
	}
	return filepath.Join(home, ".local", "share")
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
