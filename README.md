# sb

Second Brain control plane. TUI for managing WORK.md files across projects — brain dump ideas and clean up backlogs with a local LLM.

## Quick Install

Supported platforms: Linux and macOS. On Windows, use WSL.

Recommended (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/LFroesch/sb/main/install.sh | bash
```

Or download a binary from [GitHub Releases](https://github.com/LFroesch/sb/releases).

Or install with Go:

```bash
go install github.com/LFroesch/sb@latest
```

Or build from source:

```bash
make install
```

Command:

```bash
sb
```

## Config

`~/.config/sb/config.json` is created on first run:

```json
{
  "provider": "ollama",
  "providers": {
    "ollama": {
      "type": "ollama",
      "model": "qwen2.5:7b",
      "base_url": "http://localhost:11434"
    }
  },
  "scan_roots": [
    { "name": "projects", "path": "~/projects" }
  ],
  "file_patterns": ["WORK.md"],
  "idea_dirs": [],
  "label_max_depth": 2,
  "index_path": "~/.config/sb/index.md",
  "log_level": "info",
  "catchall_target": null,
  "ideas_target": null
}
```

| Field | Purpose |
|-------|---------|
| `provider` | Active provider profile name from the `providers` map. |
| `providers` | Named LLM profiles. Each entry supports `type`, `model`, `base_url`, `api_key`, `api_key_env`, and optional extra `headers`. |
| `scan_roots` | Recursive discovery roots. `name` is used only when sb needs extra label context. |
| `file_patterns` | Filenames sb treats as projects (e.g. add `ROADMAP.md`). |
| `idea_dirs` | Flat dirs whose `.md` files are loaded directly (no recursion). |
| `label_max_depth` | Fallback label depth: keep the last N path components when no title label is present. |
| `index_path` | Auto-regenerated routing-context cache (see below). |
| `log_level` | JSON log verbosity for `~/.local/share/sb/logs/sb.log` (`debug`, `info`, `warn`, `error`). |
| `catchall_target` | `{ "name": "...", "path": "..." }` — optional bucket for general notes that don't belong to a project. |
| `ideas_target` | Same shape — optional bucket for project-less ideas. |

Press `,` inside sb to open the config in your editor.

Use `api_key_env` when possible so secrets stay out of the config file. Existing top-level `model` and `ollama_host` fields are still honored as a compatibility shim for older configs.

Env overrides:

- `SB_PROVIDER` selects the active named profile.
- `SB_MODEL`, `SB_BASE_URL`, `SB_API_KEY`, `SB_API_KEY_ENV`, and `SB_PROVIDER_TYPE` override the active profile.
- `OLLAMA_HOST` still overrides the active profile's base URL when the provider type is `ollama`.

Example multi-provider setup:

```json
{
  "provider": "openai",
  "providers": {
    "ollama": {
      "type": "ollama",
      "model": "qwen2.5:7b",
      "base_url": "http://localhost:11434"
    },
    "openai": {
      "type": "openai",
      "model": "your-openai-model",
      "base_url": "https://api.openai.com/v1",
      "api_key_env": "OPENAI_API_KEY"
    },
    "anthropic": {
      "type": "anthropic",
      "model": "your-anthropic-model",
      "base_url": "https://api.anthropic.com",
      "api_key_env": "ANTHROPIC_API_KEY"
    }
  }
}
```

The `o` key opens project directories in your editor — set `$VISUAL` (GUI) or `$EDITOR` (terminal). Falls back to probing for cursor, code, nvim, vim, nano.

```bash
export EDITOR=nvim      # terminal editor
export VISUAL=cursor    # GUI editor (checked first)
```

The header now shows the active LLM profile and model, for example `llm=openai:gpt-4.1-mini`. If the selected provider is incomplete, sb shows a warning like `llm disabled (missing OPENAI_API_KEY)` instead of leaving the active backend ambiguous.

### Project labels and descriptions

Any discovered markdown file can define both its dashboard label and routing description from the first H1:

```markdown
# WORK - sb | Second Brain TUI for managing WORK.md files across projects
# ROADMAP - toolkit | v1 polish
```

`label` overrides the left-panel project name. `description` feeds the router and index. If there is no title label, sb falls back to the shortest useful relative path within the scan root, expanding only when needed to resolve collisions.

If two different roots still collide after path expansion, sb prefixes the fallback label with the root name, e.g. `work/api` and `client/api`.

### Index file

On every startup sb writes `~/.config/sb/index.md` — a human-readable list of every discovered project + its description, plus the configured special targets. It's a **read-only artifact** for inspecting what the active model sees during routing. Edits get overwritten on the next startup; change the markdown title line to update a label or description.

### Logging

sb now logs structured JSON to `~/.local/share/sb/logs/sb.log` and rotates at roughly 5 MiB, keeping 3 backups. The old `/tmp/sb-*.log` files are no longer used.

---

## Workflows

### Brain Dump (`d`)

Offload thoughts without deciding where they go. sb asks the active LLM provider to classify each item and route it to the right project's WORK.md.

1. Press `d` from the dashboard
2. Type anything — a task, idea, or note (multi-line ok)
3. `ctrl+d` to route — the active model splits and classifies each item
4. Step through items one by one:
   - `y`/`enter` — accept, write to target project
   - `n` — skip item
   - `r` — reroute: type a hint and the model re-classifies the item
   - `esc` — abort remaining items (already accepted ones are kept)
5. If the model is unsure about a project, a clarify prompt appears automatically — type a hint and press `enter` to reroute, or `esc` to skip

After stepping through all items, a summary shows accepted vs skipped.

### Cleanup (`c` / `C`)

Normalize and condense a WORK.md file via the active LLM provider — removes duplicates, fixes formatting, collapses redundant entries.

- `c` — clean up the currently selected/viewed project
- `C` — chain cleanup: runs on all selected projects (use `space` to select, or all if none selected)

After the model runs, you see a diff. Press `y` to accept, `n` to reject, or `f` to give feedback and regenerate.

### Daily Plan (`P`)

Select projects with `space`, then press `P`. The active model reads their tasks and generates a prioritized daily plan.

### Next Todo (`t`)

Press `t` on any project — the active model reads the WORK.md and tells you the single best thing to work on next.

### Fix non-list lines (`-`)

Scans WORK.md for lines that aren't proper list items and fixes them in-place. Useful after messy manual edits.

---

## Navigation

| Key | Action |
|-----|--------|
| `j/k` | Navigate |
| `enter` | Open project |
| `e` | Edit WORK.md inline |
| `d` | Brain dump |
| `c` / `C` | Cleanup (single / chain) |
| `P` | Daily plan |
| `t` | Next todo |
| `/` | Search across all WORK.md files |
| `f` | Pin/unpin project |
| `space` | Toggle project selection |
| `o` | Open project dir in editor |
| `r` | Refresh |
| `,` | Open `config.json` in editor |
| `?` | Help |

Full keybind reference is available in-app with `?`.

---

## Design Docs

- [docs/agent-cockpit-rfc.md](docs/agent-cockpit-rfc.md) — product direction for turning `sb` into the first cockpit UI for coding-agent orchestration, with a separable foreman/runtime layer behind it

## License

[AGPL-3.0](LICENSE)
