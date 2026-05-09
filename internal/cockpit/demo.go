package cockpit

import (
	"errors"
	"os"
	"strings"
)

const DemoModeLLMDisabledNote = "public demo mode — LLM conversations are disabled in the browser demo"

func IsDemoMode() bool {
	return os.Getenv("DEMO_ENV") == "1" || os.Getenv("TUI_HUB_DEMO") == "1"
}

func IsDemoDisabledNote(note string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(note)), "public demo mode")
}

func demoModeExecutorError(spec ExecutorSpec) error {
	if !IsDemoMode() {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(spec.Type), "ollama") {
		return errors.New(DemoModeLLMDisabledNote)
	}
	return nil
}
