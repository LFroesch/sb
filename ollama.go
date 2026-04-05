package main

import (
	"context"
	"fmt"

	"github.com/LFroesch/sb/internal/ollama"
	"github.com/LFroesch/sb/internal/workmd"
)

var ollamaClient = ollama.New()

// routeWithOllama classifies a brain dump text and returns the target project name + section.
func routeWithOllama(text string, projects []workmd.Project) (string, string, error) {
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000) // 30s
	defer cancel()

	result, err := ollamaClient.Route(ctx, text, names)
	if err != nil {
		return "", "", fmt.Errorf("route: %w", err)
	}

	return result.Project, result.Section, nil
}
