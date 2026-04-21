# DEVLOG

## 2026-04-20

- Rewrote [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) into a tighter build RFC: sharp `v1` scope, shared day/night job model, PTY-backed worker sessions, master-session control, concise `v2` notes, and a clearer `task -> launch preset -> job` backbone.
- Added a top-of-RFC warning plus a hidden-requirements checklist so implementation starts by locking authority, state, lifecycle, safety, defaults, artifacts, and recovery behavior.
- Added provider-oriented LLM config with named profiles plus compatibility fallback for older top-level `model` / `ollama_host` settings.
- Replaced the Ollama-only client with `internal/llm`, keeping the existing prompts while adding OpenAI and Anthropic HTTP adapters.
- Switched UI/help copy from provider-specific Ollama wording to generic model wording and added config tests in `internal/config/config_test.go`.
- Added header-visible provider status so the active LLM profile/model is always visible and incomplete setups surface as `llm disabled (...)`.
- Replaced `scan_dirs`-only fallback naming with named `scan_roots` plus collision-aware relative-path labels for users with many same-named files like `WORK.md`.
- Kept title-driven labels for any discovered markdown file via `# TYPE - label | description`, and now disambiguate duplicate title labels by appending the shortest useful relative suffix.
- Replaced hard-coded `/tmp/sb-*.log` files with JSON `slog` output under `~/.local/share/sb/logs/sb.log` using built-in size rotation.
- Added naming tests in `internal/workmd/naming_test.go`.
- Added [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md), an RFC for using `sb` as the first cockpit UI for coding-agent orchestration while keeping foreman/runtime concerns outside the TUI process.
