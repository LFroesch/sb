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

## Config

`~/.config/sb/config.json` is created on first run:

```json
{
  "model": "qwen2.5:7b",
  "ollama_host": "http://localhost:11434"
}
```

Env vars `SB_MODEL` / `OLLAMA_HOST` override the config file.

## Roadmap

- **Multi-provider LLM support** — `provider` + `api_key` fields in config to support Anthropic/OpenAI alongside Ollama. Prompts are already provider-agnostic; needs a common interface + per-provider HTTP client.
