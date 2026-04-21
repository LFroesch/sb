# DEVLOG

## 2026-04-20

- Replaced `scan_dirs`-only fallback naming with named `scan_roots` plus collision-aware relative-path labels for users with many same-named files like `WORK.md`.
- Kept title-driven labels for any discovered markdown file via `# TYPE - label | description`, and now disambiguate duplicate title labels by appending the shortest useful relative suffix.
- Replaced hard-coded `/tmp/sb-*.log` files with JSON `slog` output under `~/.local/share/sb/logs/sb.log` using built-in size rotation.
- Added naming tests in `internal/workmd/naming_test.go`.
- Added [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md), an RFC for using `sb` as the first cockpit UI for coding-agent orchestration while keeping foreman/runtime concerns outside the TUI process.
