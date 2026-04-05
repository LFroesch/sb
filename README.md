# sb

Second Brain control plane. TUI for managing WORK.md files, brain dumping ideas, and running maintenance scripts.

## Install

```bash
cd sb && make install
```

## Usage

```bash
sb              # launch TUI
```

## Features

- **Dashboard** — all WORK.md files at a glance with task counts
- **Project viewer** — rendered markdown with inline editing
- **Brain dump** — type a thought, ollama routes it to the right project
- **Scripts** — run maintenance scripts (knowledge-index, obsidian-sync, workmd-audit, devlog-split, etc.)

## Environment

- `OLLAMA_HOST` — ollama API (default: localhost:11434)
- `SB_MODEL` — model for brain dump routing (default: qwen2.5:7b)
