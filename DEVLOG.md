# DEVLOG

## DevLog

### 2026-05-01 — Foreman/provider policy now reaches Claude and Codex
- Fixed the Foreman/provider launch gap where `sb` stored preset permissions and queue-only state on the job, but did not translate that policy into real Claude/Codex CLI flags at launch time. Claude jobs now emit explicit `--permission-mode` values (`dontAsk` for unattended queued work, `bypassPermissions` for `wide-open`), and Codex jobs now emit explicit sandbox / approval flags plus `--cd` for the job repo. Files: [`internal/cockpit/manager.go`](internal/cockpit/manager.go), [`internal/cockpit/runner_tmux.go`](internal/cockpit/runner_tmux.go), [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go).
- Updated the work backlog and user docs to reflect the fix and the remaining follow-up around task-backed launches still locking the working root to the task repo. Files: [`WORK.md`](WORK.md), [`README.md`](README.md).
- Why: Foreman runs were still falling back to provider-default interactive permission prompts, which broke the unattended queue contract and surfaced bogus "do you want me to do what you asked?" confirmations.

### 2026-04-30 — New-run Foreman toggle moved off `F`, queued runs get Foreman protocol
- Moved the New Run composer’s launch-mode toggle from plain `F` to `ctrl+t`, updated the footer/help copy, and let the Note textarea receive a literal uppercase `F` again instead of hijacking it for Foreman mode. Files: [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/view.go`](internal/tui/view.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go), [`README.md`](README.md).
- Foreman-managed launches now append an extra `FOREMAN PROTOCOL` block to the composed executor prompt so queued unattended runs get the stricter iterate-until-complete / no-permission-ask instruction automatically. Files: [`internal/cockpit/hooks.go`](internal/cockpit/hooks.go), [`internal/cockpit/manager.go`](internal/cockpit/manager.go), [`internal/cockpit/presets_test.go`](internal/cockpit/presets_test.go).
- Why: `F` in the note field made normal typing brittle, and `ctrl+f` would have been an even worse collision because operators commonly use it as search muscle memory.

### 2026-04-30 — Foreman supervisor fallback now yields interrupted / limit-blocked tmux jobs
- Hardened tmux supervisor state detection so Foreman no longer relies exclusively on explicit `SB_STATUS:*` markers. If the live pane ends with strong fallback phrases like `conversation interrupted` or `usage limit reached`, the job now yields back to `waiting for input` instead of looking permanently `working` and blocking the repo queue. Files: [`internal/cockpit/runner_tmux.go`](internal/cockpit/runner_tmux.go), [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go).
- Updated the user docs and work backlog to describe the new fallback behavior and note that additional provider-specific phrases may still be worth teaching the supervisor over time. Files: [`README.md`](README.md), [`WORK.md`](WORK.md).
- Why: unattended Foreman queues were only as robust as upstream CLIs' willingness to print exact supervisor markers; common interruption / limit messages needed a practical recovery path so one stuck session would not hold a repo lock forever.

### 2026-04-30 — Indexing metadata refresh: typed H1 plus summary line, richer index, lighter cleanup
- Changed `sb` project metadata parsing so indexed markdown files now prefer a typed first heading plus the next plain-text summary line, e.g. `# WORK - sb` followed by a short description. Legacy inline metadata like `# WORK - sb | description` still parses for compatibility. Files: [`internal/workmd/workmd.go`](internal/workmd/workmd.go), [`internal/workmd/naming_test.go`](internal/workmd/naming_test.go).
- Enriched discovered project context with optional `Current Phase` text plus a tiny active-task preview, then used that in the generated index and brain-dump routing prompt. The router path is intentionally capped to compact fields so local Ollama models do not get flooded with full-file context. Files: [`internal/workmd/index.go`](internal/workmd/index.go), [`internal/llm/llm.go`](internal/llm/llm.go), [`internal/llm/llm_test.go`](internal/llm/llm_test.go), [`internal/tui/model.go`](internal/tui/model.go), [`internal/tui/update.go`](internal/tui/update.go).
- Relaxed cleanup from canonical section rewriting to a lighter “preserve structure, fix obvious duplication/formatting” contract so small local models are less likely to thrash headings. Files: [`internal/llm/llm.go`](internal/llm/llm.go), [`README.md`](README.md), [`WORK.md`](WORK.md).
- Updated docs to describe the preferred metadata format, compact routing context, and lighter cleanup behavior. Files: [`README.md`](README.md), [`WORK.md`](WORK.md).
- Why: discovery was already configurable, but the router/index still relied on brittle title-only metadata and cleanup was too rigid for `qwen2.5:7b`.

### 2026-04-30 — Foreman V1 hardening pass: parallel-start coverage, normal review visibility, paused-state cleanup
- Added a positive Foreman scheduler regression test that queues two eligible jobs in different repos and proves both transition to `running` when Foreman is enabled, instead of only covering the negative concurrency-cap case. Files: [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go).
- Added an Agents-surface regression test that proves unattended results still show up through the standard list filters: Foreman `needs_review` jobs remain visible under `attention`, and Foreman `completed` jobs remain visible under `done`. Files: [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).
- Removed the dead V1 `paused` status from the runtime/UI path so `Esc` is treated as a literal tmux key send plus note text, not as a reserved lifecycle state that could imply the dashboard supports real pausing already. Files: [`internal/cockpit/types.go`](internal/cockpit/types.go), [`internal/cockpit/registry.go`](internal/cockpit/registry.go), [`internal/cockpit/runner_tmux.go`](internal/cockpit/runner_tmux.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`README.md`](README.md), [`WORK.md`](WORK.md), [`docs/foreman-night-mode.md`](docs/foreman-night-mode.md).
- Why: these were the remaining pre-V1 confidence gaps from the readiness pass, and the docs/work backlog needed to reflect that Foreman V1 is now covered by explicit tests rather than inference.

### 2026-04-29 — Global `,` now opens the whole sb config directory
- Changed the global `,` binding from “open `config.json`” to “open the main `sb` config directory” so manual editing of `config.json`, presets, providers, prompts, and hooks is available from anywhere in the app through the same shortcut. Files: [`internal/config/config.go`](internal/config/config.go), [`internal/config/config_test.go`](internal/config/config_test.go), [`internal/tui/update.go`](internal/tui/update.go), [`internal/tui/view.go`](internal/tui/view.go), [`README.md`](README.md), [`WORK.md`](WORK.md).
- Why: opening just the single config file was too narrow once more of the runtime setup moved into the managed JSON library under `~/.config/sb`.

### 2026-04-29 — Agent Setup edits refresh immediately and ID edits behave like renames
- Fixed the in-app Agent Setup save path so edited preset/provider/prompt/hook-bundle fields now stay selected after the catalog reload instead of risking a jump back onto stale data. The save flow now reselects the edited item by ID after refresh. Files: [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- ID edits now behave like real renames: after saving a new managed-item ID, the old `<id>.json` file is removed instead of being left behind as a stale duplicate that can reappear on the next reload.
- Preset summaries in Agent Setup now show the current prompt / hook bundle / engine references directly, so composition edits are visible immediately instead of only indirectly through resolved runtime fields. Files: [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- The in-place enum-cycle path (`enter` on preset prompt / hook bundle / engine refs and similar fields) now uses the same post-save refresh logic as manual text edits, so the selected item re-resolves and repaint happens immediately after each cycle step. Files: [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- Removed the static `↻ option1/option2/...` decoration from Agent Setup field rows. It was only explanatory copy, not live state, and it made cycling fields look broken when the hint itself did not change. Files: [`internal/tui/view_agent.go`](internal/tui/view_agent.go).
- Why: Agent Setup could look like a save "didn't take" because the post-save reload could leave the UI pointed at stale data, and explicit ID changes could accumulate duplicate files on disk.

### 2026-04-29 — Custom repo-path editor stays visible on shorter terminals
- Fixed the freeform Agent launch Repo step so the inline `(custom path…)` editor now reserves its own visible space instead of getting pushed below the pane by the repo list. While the editor is active, the input renders ahead of the list and the extra repo-step subtitle collapses so the typed path stays visible. Files: [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).
- Clamped the custom repo-path input width to the actual pane width instead of forcing a 40-column minimum, and updated the launch flow / resize handlers to keep that width synced on open and window resize. Files: [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/update.go`](internal/tui/update.go).
- Why: on shorter or narrower terminals, the active custom-path field could render below the visible body area or wrap badly enough that you could not see what you were typing.

### 2026-04-29 — Agent Setup pruning: nav fixes, active-panel outline, shell removed
- Agent Setup detail navigation now matches the rest of the app: `h` / `←` returns to the list panel from a field detail, `l` / `→` enters detail from the list, `tab` from the last group wraps back to the list instead of looping, `shift+tab` continues to step backward. The focused side renders with `panelActiveStyle` so it's obvious which panel owns the cursor. Files: [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go), [`internal/tui/view_agent_manage.go`](internal/tui/view_agent_manage.go).
- Removed cruft fields from the Agent Setup field list: presets lost the duplicate `role` field (was just `Name` rendered as a dim subtitle), engines lost `executor.runner` / `executor.cmd` / `executor.args`, hook bundles lost `iteration.mode` (only `one_shot` is implemented in V0). Field getters/setters and selected-summary lines updated to match. Files: [`internal/cockpit/types.go`](internal/cockpit/types.go), [`internal/cockpit/presets.go`](internal/cockpit/presets.go), [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go).
- Removed user-facing **shell** support: dropped the `shell` provider seed, the `shell-escape` preset, the `shell-escape` prompt template, and the `shell-test`/`shell-lint`/`shell-build`/`shell-escape` cases from the preset sort rank. README seed lists updated. The internal `shell` engine case in [`manager.go`](internal/cockpit/manager.go) stays as a substrate the test suite uses for end-to-end exec; it's just not exposed as a configurable choice anymore. Files: [`internal/cockpit/providers.go`](internal/cockpit/providers.go), [`internal/cockpit/presets.go`](internal/cockpit/presets.go), [`internal/cockpit/prompts.go`](internal/cockpit/prompts.go), [`README.md`](README.md), [`internal/cockpit/presets_test.go`](internal/cockpit/presets_test.go).
- Why: the wizard exposed knobs that didn't drive runtime behavior (cmd/args/runner only matter for shell, iteration.mode only honors one value, role duplicated name) and shell escape was clutter Lucas isn't using.

### 2026-04-29 — Repo picker defaults to row 2 + launch page shows longer repo paths
- Adjusted the freeform Agent launch Repo picker so `(custom path…)` now stays at the top of the list while the initial selection still lands on the next row, which keeps the normal repo as the default and makes custom-path entry a single `↑` away. The custom-path editor still opens with `enter` on that sentinel and typed keys continue to route into the inline input. Files: [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- Reworked the launch-page repo rendering so repo paths are shown much longer there instead of using the global 20-character tail clip; the launch summary and repo list now render readable full/home-relative paths and rely on wrapping instead of heavy truncation. Files: [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go), [`README.md`](README.md), [`WORK.md`](WORK.md).
- Why: the old ordering hid the intended default-vs-custom behavior, and the launch page’s ultra-short repo labels made different targets indistinguishable.

### 2026-04-29 — Repo chooser order stabilized + custom-path typing routed correctly
- Fixed the freeform Agent launch Repo picker so it no longer snaps the selection back toward the top while you scroll. The repo menu now keeps a stable order, leaves `(custom path…)` reachable at the end, and still appends any non-discovered active repo only after the discovered/cwd entries. Also fixed the top-level key router so once the inline custom-path editor is open, typed characters actually reach that `textinput`. Files: [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/update.go`](internal/tui/update.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- Why: the old menu builder kept re-inserting the current selection near the top on every move, which made scrolling feel broken and effectively hid the custom-path option.

### 2026-04-29 — Agent width wrapping pass + queued tmux status fix
- Replaced several single-line truncation paths in the Agent UI with width-aware wrapping against the actual pane width. `view.go` now exposes shared ANSI-safe wrap helpers, `view_agent.go` wraps the filter/counts/session header rows, `view_agent_launch.go` no longer hard-caps source summaries at 42 columns, and `view_agent_attached.go` wraps header/meta/source rows instead of clipping them early.
- Tightened the attached tmux-open path so queued/deferred tmux jobs no longer report the bogus `tmux window not recorded for this job` message before dispatch. The TUI now surfaces queue/defer reasons (`waiting for foreman`, `repo busy`, etc.) for queued tmux jobs and uses a softer `tmux session still initializing` message for short-lived attach races. ([`internal/tui/update_agent_attached.go`](internal/tui/update_agent_attached.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go))
- Added `ctrl+g` to the attached-session footer/help/readme hints so the visible in-app controls now mention the tmux-level jump back to the shared `sb` dashboard window. ([`internal/tui/view.go`](internal/tui/view.go), [`README.md`](README.md))

### 2026-04-29 — Repo step now confirms before launch
- Adjusted the freeform Agent launch composer so `enter` on the **Repo** tab no longer fires a launch immediately. It now confirms the selected repo and advances focus into the **Note** editor; selecting `(custom path…)` still opens the inline path input first, and pressing `enter` there now commits the path and moves into the note step. Files: [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go), [`README.md`](README.md).
- Why: the old behavior made the repo chooser feel broken because the same key used to pick a repo was interpreted as "launch now" before the user had a chance to confirm the selection or type a custom path.

### 2026-04-29 — Agent run picker freeform sentinel + Setup wizard
- Replaced the `n` / `N` split in the Agents tab with a single `n` entry. Step 1 of the picker now renders `★ New run without task source` as row 0 ([`internal/tui/view_agent_picker.go`](internal/tui/view_agent_picker.go), [`internal/tui/update_agent_picker.go`](internal/tui/update_agent_picker.go)); selecting it drops you into the launch composer with the **Repo** tab focused so the run target is an explicit choice instead of an implicit cwd guess. Selecting a real project keeps the existing task multi-select flow.
- Added a `(custom path…)` sentinel to the Repo tab options and an inline textinput (`launchRepoCustom`, `launchRepoEditing` in [`internal/tui/model.go`](internal/tui/model.go)) so users can type any absolute path — repos that aren't tracked by sb included. Pressing enter on the sentinel opens the editor; enter again commits the path. ([`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go))
- Removed the `N`, `p`, `v`, `P`, `V` keybinds from the Agents list along with the dead `createPresetTemplate` / `createProviderTemplate` helpers ([`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go)). Help table and README updated to match.
- Reframed Agent Setup (`m`) as a step-based wizard. Added `agentManageGroup` / `agentManageAdvanced` / `agentManageWizard` model state and helpers `agentManageGroupOrder`, `agentManageGroupFieldIndices`, `agentManageEnsureGroupField`, `agentManageCycleGroup` in [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go). The right pane renders a single group at a time (Identity → Prompting → Suggested Engine → Iteration); `tab` cycles groups, `j/k` walks fields within the visible group, `pgup/pgdn` also cycles groups, and `a` reveals the previously hidden Hooks (raw JSON) and Advanced groups. Iteration mode conditionally exposes `n` / `signal` / `on_file` only when relevant.
- Added enum cyclers: pressing enter on `Permissions`, `Iteration mode`, or `Launch mode` rotates through the fixed option list instead of opening the editor (`enumOptionsForFieldKey` / `cycleEnumValue`). Pressing `n` to create a new role/engine starts the wizard with `Name` already in the editor and auto-advances groups on each save until the wizard finishes.
- Renamed user-facing "freeform" wording: review pane now says `no task source`, help dropdown lost the freeform row, README's `n`/`N` section was collapsed into one entry that documents the freeform sentinel. Internal struct fields (`Job.Freeform`) are unchanged so on-disk job records keep loading.
- Added eligibility gating to the unattended scheduler in [`internal/cockpit/manager.go`](internal/cockpit/manager.go): `tickScheduler` now walks queued jobs and records the first failing gate (waiting for foreman, repo busy, foreman concurrency cap, provider near 5h limit) on the job as `EligibilityReason` instead of silently skipping. Configurable via two new `ForemanState` fields, `MaxConcurrent` (default 3) and `LimitGuardPct` (default 90).
- Added a 30s background ticker so quota resets pick up parked work without operator nudges, and made `LaunchJob` always tick so QueueOnly launches surface their reason immediately.
- Surfaced the new state in the Agents list ([`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/update_agent.go`](internal/tui/update_agent.go)): the foreman header now shows `pool N parked · N eligible · N deferred · N/N active`, the status column reads `deferred` (warn) when a reason is set, and the right-side peek includes a `deferred` row with the reason verbatim. Live/attention filters now include deferred jobs.
- Updated the launch composer copy in [`view_agent.go`](internal/tui/view_agent.go) so a queue-only launch with foreman off shows a one-line hint about parking in the pool until `F` flips foreman on.
- Refocused [`WORK.md`](WORK.md) §5 on the V1 foreman pool surface and split the bulk multi-repo prep aspiration into a §5.x post-V1 backlog item.
- Added gate coverage in [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go) (waiting-for-foreman, repo-busy, concurrency cap, claude near-limit, ollama/shell skip) and TUI snapshots in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go) (pool counters, deferred status, deferred peek row). `go test ./internal/cockpit ./internal/tui` passes.

### 2026-04-28 — tmux statusline: 7d reset date, codex/claude color split, `|` divider
- Added a 7-day reset marker to the tmux statusline in [`internal/statusbar/tmux.go`](internal/statusbar/tmux.go) using the new `tmuxShortDate` helper (M/D format) so each provider block now shows both the 5h and 7d reset markers when the window data is populated.
- Split the provider label color: claude stays `#0099ff`, codex now renders in `#10a37f` so the two blocks read as distinct at a glance.
- Switched the inter-provider divider from `·` to `|`.
- Extended [`internal/statusbar/tmux_test.go`](internal/statusbar/tmux_test.go) with a 7d-reset assertion, a per-provider color assertion, and a `tmuxShortDate` format check. `go test ./internal/statusbar` passes.

### 2026-04-28 — Agent layout budget cleanup for short terminals
- Reworked the Agent-page sizing in [`internal/tui/view.go`](internal/tui/view.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/view_agent_picker.go`](internal/tui/view_agent_picker.go), [`internal/tui/view_agent_manage.go`](internal/tui/view_agent_manage.go), and [`internal/tui/view_agent_attached.go`](internal/tui/view_agent_attached.go) so all subviews size themselves against the actual visible body area after the global header/footer/status chrome is accounted for.
- This fixes the new-run composer being too tall and the related picker / setup / attached-session clipping where local content could push the global header or footer off-screen instead of scrolling inside the active view.
- Added short-terminal regressions in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go) covering launch, picker, setup, list, and attached-session rendering. Verified with `go test ./internal/tui ./internal/cockpit`.
- Followed up by clamping stored scroll offsets in [`internal/tui/update.go`](internal/tui/update.go), [`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go), [`internal/tui/view.go`](internal/tui/view.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), and [`internal/tui/view_agent_manage.go`](internal/tui/view_agent_manage.go) so paging past the end no longer leaves review/setup/detail panes logically off-screen. Added matching regressions in [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go).
- Removed the repeated in-body keybinding copy from the Agent picker / launch / attached / setup surfaces plus the main cleanup / dump review bodies, keeping control hints in the footer instead. Updated the matching Agent render expectations in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).
- Tightened the specific pages that still felt wrong after that first pass: `Pick tasks`, `New Run` / `Review Run`, and the in-app Agent Setup field editor now all use bounded windowing tied to the actual visible rows, while the Agents-list right detail pane was simplified into a compact operator summary instead of trying to inline the full review surface.

### 2026-04-28 — Role editor cleanup: fewer duplicate identity fields, auto-generated IDs
- Simplified the Agent Setup field ordering in [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go): `Name` now leads, core run settings stay near the top, and `persona / role` plus `file ID` are pushed into an `Advanced` section instead of dominating the form.
- Added automatic ID generation from the visible name for both roles and engines, so creating or renaming an item no longer requires manually maintaining a slug-like file ID unless you want to override it.
- Tightened the launch/setup summaries in [`internal/tui/view_agent.go`](internal/tui/view_agent.go) so they stop repeating duplicate role/persona information when the stored slug and visible name are effectively the same thing.
- Added a regression in [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go) and verified with `go test ./internal/tui ./internal/cockpit`.

### 2026-04-28 — Agent list UX cleanup: natural peek paging, clearer stopped/closed tmux states, leaner role seeds
- Fixed the Agent jobs list paging in [`internal/tui/update_agent.go`](internal/tui/update_agent.go) so `pgup` / `pgdn` now scroll the right-side peek in the expected direction instead of feeling inverted against the bottom-anchored log view.
- Tightened status rendering in [`internal/tui/view_agent.go`](internal/tui/view_agent.go) and [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go): interrupted tmux jobs now read as `stopped`, closed tmux windows now read as `closed`, and those states no longer masquerade as ordinary `waiting for input`.
- Decluttered preset loading/seeding in [`internal/cockpit/presets.go`](internal/cockpit/presets.go) and [`internal/cockpit/presets_test.go`](internal/cockpit/presets_test.go): new installs get a smaller core role set, while older utility roles still load but sort below the main coding roles.
- Updated the user-facing Agent notes in [`README.md`](README.md) and the current-product notes in [`WORK.md`](WORK.md). Verified with `go test ./internal/tui ./internal/cockpit`.

### 2026-04-28 — Agent wording cleanup: queue vs status, roles vs engines
- Tightened the Agent launch/list/setup wording in [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/view_agent_manage.go`](internal/tui/view_agent_manage.go), and [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go).
- The jobs table now labels queue progress as `queue` instead of `advance`, the new-run composer now reads as `Role` + `Engine` instead of `Template` + `Runtime`, and the review pane now shows the effective engine once instead of repeating runtime information as if it were two separate choices.
- Updated the matching TUI regression in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go), plus the user-facing Agent docs in [`README.md`](README.md) and the current-product notes in [`WORK.md`](WORK.md).

### 2026-04-28 — Explicit tmux yield signaling for human input / review
- Added a supervisor marker protocol to tmux-backed launch prompts in [`internal/cockpit/hooks.go`](internal/cockpit/hooks.go): jobs are now instructed to emit `SB_STATUS:WAITING_HUMAN` when they need the operator to respond, and `SB_STATUS:READY_REVIEW` when they are done and ready for review.
- Updated the tmux runner in [`internal/cockpit/runner_tmux.go`](internal/cockpit/runner_tmux.go) plus manager wiring in [`internal/cockpit/manager.go`](internal/cockpit/manager.go) so live pane snapshots are scanned for those markers. When seen, the job is transitioned into a real yielded state (`StatusIdle` with note `waiting for human input`, or `StatusNeedsReview` with note `ready for review`) without killing the tmux session.
- This gives both normal jobs and Foreman a real state transition to observe instead of relying on the fake proxy of "tmux window is still alive". Added regressions in [`internal/cockpit/presets_test.go`](internal/cockpit/presets_test.go) and [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go). `go test ./...` passes.

### 2026-04-28 — Agent surface simplification follow-up
- Tightened the user-facing Agent model further in [`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/update_agent_attached.go`](internal/tui/update_agent_attached.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_attached.go`](internal/tui/view_agent_attached.go), [`internal/tui/view.go`](internal/tui/view.go), and the matching tests.
- Removed the remaining dead top-level `retry` key path from the main Agent list flow, kept `R` focused on "start waiting job now or open the selected session", and tightened the docs/help text so sourced launches, Foreman waiting jobs, and literal session controls read more directly.
- Updated [`WORK.md`](WORK.md) and [`README.md`](README.md) to match the simpler model: normal jobs plus optional Foreman waiting state, not a second parallel job concept.

### 2026-04-28 — Honest session controls: Esc / Ctrl+C / continue
- Replaced the fuzzy top-level Agent control language with literal session actions in [`internal/cockpit/iface.go`](internal/cockpit/iface.go), [`internal/cockpit/protocol.go`](internal/cockpit/protocol.go), [`internal/cockpit/client.go`](internal/cockpit/client.go), [`internal/cockpit/server.go`](internal/cockpit/server.go), [`internal/cockpit/manager.go`](internal/cockpit/manager.go), [`internal/cockpit/tmux.go`](internal/cockpit/tmux.go), and [`internal/cockpit/runner_tmux.go`](internal/cockpit/runner_tmux.go).
- `s` is now a soft stop for tmux-backed sessions (`Esc`), `S` is a hard interrupt (`Ctrl+C`), and `c` literally sends `continue`. Waiting Foreman jobs can also be started immediately with `R` instead of pretending they are a separate stuck job type.
- Updated the Agent TUI surfaces and tests in [`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/update_agent_attached.go`](internal/tui/update_agent_attached.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_attached.go`](internal/tui/view_agent_attached.go), [`internal/tui/view.go`](internal/tui/view.go), [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go), and [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).
- Why: the old `stop/resume/retry` story was overstating what `sb` can actually control over foreign tmux-hosted CLIs. These controls now describe the real mechanism instead of a fake semantic layer.

### 2026-04-28 — Launch flow simplification: sourced runs no longer ask for repo
- Simplified the Agent new-run composer in [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/update.go`](internal/tui/update.go), and the matching tests in [`internal/tui/update_agent_test.go`](internal/tui/update_agent_test.go) / [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).
- Task-backed runs now inherit repo directly from the selected task and skip the extra Repo step. Only freeform runs still expose explicit repo selection.
- Why: the old composer was asking the user to restate information the task picker already knew, which made the launch flow feel more complicated than the product actually needs.

### 2026-04-28 — Foreman priority + simplification note
- Recorded the current product correction in [`WORK.md`](WORK.md): Foreman behavior is the active priority, turning Foreman off should leave those runs as ordinary jobs instead of a special stranded type, and the current preset/template/shell surface is acknowledged as too complicated but explicitly deferred behind Foreman correctness.
- Added the sharper mental model there too: Foreman is "hold this until later, then run it when eligible" on top of the same normal jobs, with repo safety / limit checks / AFK policy acting as scheduling constraints rather than a different product flow.
- This is a scope-control note so future work stops drifting back into setup-page/preset churn while the unattended execution model is still unsettled.

### 2026-04-28 — Foreman model correction: prepared job pool, not NightEligible queueing
- Corrected the product/docs model for Foreman in [`docs/foreman-night-mode.md`](docs/foreman-night-mode.md), [`docs/product-definition.md`](docs/product-definition.md), [`WORK.md`](WORK.md), and [`README.md`](README.md): Foreman is now defined as a pool of prepared unattended jobs that can launch in separate sessions when eligible, not a special `NightEligible` preset flag or a strictly serial night-only queue.
- Removed the stale `NightEligible` field from [`internal/cockpit/types.go`](internal/cockpit/types.go) and from the in-app Agent Setup editor in [`internal/tui/update_agent_manage.go`](internal/tui/update_agent_manage.go) / [`internal/tui/view_agent.go`](internal/tui/view_agent.go).
- The corrected mental model is: start now or send to Foreman, let different repos run in parallel when safe, serialize same-repo writes, then review the resulting jobs in the normal Agents surface later.

### 2026-04-28 — First Foreman implementation slice: persisted on/off + queue-for-Foreman launches
- Added a real persisted Foreman state to the cockpit layer in [`internal/cockpit/types.go`](internal/cockpit/types.go), [`internal/cockpit/foreman_state.go`](internal/cockpit/foreman_state.go), [`internal/cockpit/manager.go`](internal/cockpit/manager.go), [`internal/cockpit/iface.go`](internal/cockpit/iface.go), [`internal/cockpit/protocol.go`](internal/cockpit/protocol.go), [`internal/cockpit/client.go`](internal/cockpit/client.go), and [`internal/cockpit/server.go`](internal/cockpit/server.go).
- Foreman can now be toggled on/off from the TUI, the state survives restarts, and queued work marked `wait for Foreman` stays parked until Foreman is enabled.
- Added a queue-for-Foreman launch path in [`internal/tui/update_agent_launch.go`](internal/tui/update_agent_launch.go) and [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go): pressing `F` in the new-run composer flips the launch between immediate start and queued-for-Foreman.
- Surfaced Foreman status and controls in the Agents UI through [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/update_agent.go`](internal/tui/update_agent.go), [`internal/tui/model.go`](internal/tui/model.go), [`internal/tui/run.go`](internal/tui/run.go), and [`internal/tui/update.go`](internal/tui/update.go).
- Added distinct Foreman queue visibility in the Agent list/detail UI: queued-for-Foreman jobs now read as `waiting for Foreman`, get their own `foreman` filter bucket, and show Foreman-specific status/action hints instead of looking like ordinary interactive queue backlog.
- Added manager tests proving the new seam: queue-only launches wait while Foreman is off, start when Foreman is enabled, and the Foreman state persists across manager restart in [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go). `go test ./...` passes.

### 2026-04-28 — V1 boundary corrected to include Foreman/night mode
- Reworked [`docs/product-definition.md`](docs/product-definition.md), [`docs/foreman-night-mode.md`](docs/foreman-night-mode.md), [`WORK.md`](WORK.md), and [`README.md`](README.md) so the repo no longer treats unattended Foreman/night mode as post-V1.
- The corrected product stance is: `sb` V1 is not done unless both loops work — the interactive day loop and the unattended overnight/away Foreman loop.
- This also promotes Foreman from "advanced queueing later" into an integrated core workflow with explicit requirements around queue prep, on/off control, unattended execution rules, and morning review.

### 2026-04-28 — Product-language cleanup pass across Agents/docs
- Reworked the user-facing Agent wording toward the product model in [`internal/tui/view.go`](internal/tui/view.go), [`internal/tui/view_agent.go`](internal/tui/view_agent.go), [`internal/tui/view_agent_launch.go`](internal/tui/view_agent_launch.go), [`internal/tui/view_agent_manage.go`](internal/tui/view_agent_manage.go), [`internal/tui/update_agent.go`](internal/tui/update_agent.go), and [`internal/tui/update_agent_attached.go`](internal/tui/update_agent_attached.go).
- Main changes: the page now leads with **Agents** instead of **Agent Cockpit**; **New Job** became **New Run**; `Recipe/Provider/Brief` became `Template/Runtime/Note`; review and queue actions now say `accept`, `queue`, and `skip rest of queue` instead of leaning on `approve`, `campaign`, and other operator jargon.
- Reframed the advanced management page as **Agent Setup** and relabeled its high-level buckets to **Templates** and **Runtimes** so the default product story can stay task/run/review-focused while still keeping the advanced authoring surface available.
- Rewrote [`WORK.md`](WORK.md) around a cleaner finish line and product-aligned ship blockers, and updated [`README.md`](README.md) so the main app flows read as Dashboard/Dump/Agents rather than a grab bag of implementation terms.
- Added stronger mode-local hints across the Agents screens: the jobs list now surfaces the main run controls immediately, the picker explains the sourced-run flow, the new-run composer shows focus-specific next-step hints, attached sessions call out whether transcript or input is active, and Agent Setup explicitly says it is an advanced surface.
- Updated [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go) expectations to match the new language. `go test ./...` passes.

### 2026-04-28 — Tmux status-right cleanup
- Tightened the cockpit tmux status formatter in [internal/statusbar/tmux.go](internal/statusbar/tmux.go) so any available 5h reset time renders consistently as `@time` for both Claude and Codex instead of only surfacing on some snapshots.
- Simplified the isolated session `status-right` in [internal/cockpit/tmux.go](internal/cockpit/tmux.go) to drop the redundant `sb-cockpit` / `0:main` labels and the system clock so that space is used only for the provider limits.
- Kept the tmux window list itself visible, but render window tabs as just the window name instead of `N:name` so `main` stays clickable without duplicating the numeric index.
- Added regression coverage in [internal/statusbar/tmux_test.go](internal/statusbar/tmux_test.go) and [internal/cockpit/tmux_test.go](internal/cockpit/tmux_test.go) for the reset marker and the trimmed right-side layout.

### 2026-04-28 — Recut WORK.md around a ship-today V1 boundary
- Rewrote [`WORK.md`](WORK.md) to separate `Done`, `Ship Blockers Today`, and `Post-V1` instead of mixing shipped cockpit behavior, polish passes, and future ideas into one active list.
- Locked the immediate finish line to **Agent Cockpit V1 done today**: core operator loop plus a concrete smoke pass (launch, attach, approve, skip item, skip campaign, resume, second-terminal reattach).
- Why: the feature set had become hard to reason about because the task list kept treating "usable but should feel better" as if it were the same as "not implemented". This pass is meant to restore scope control and make the stop point explicit.

### 2026-04-28 — Added product-definition source of truth
- Added [`docs/product-definition.md`](docs/product-definition.md) as the top-level product model for `sb`: Dashboard, Dump, Agents, and Foreman as one layered system instead of a pile of unrelated features.
- The doc explicitly distinguishes user-facing concepts (`project`, `task`, `dump`, `agent run`, `review`, `foreman queue`) from advanced/internal concepts (`provider`, `recipe`, `campaign`, tmux/daemon details) that should stay hidden or secondary.
- Updated [`README.md`](README.md) and [`WORK.md`](WORK.md) to point at that product definition so future cleanup work has a single place to anchor on before adding or reshaping features.

### 2026-04-27 — Split update_agent.go / view_agent.go by mode
- Sliced the two 2k-line monoliths along the same picker / launch / attached / manage seam the runtime already used. `update_agent.go` 2057 → 609 lines plus `update_agent_picker.go` (96), `update_agent_launch.go` (336), `update_agent_attached.go` (332), `update_agent_manage.go` (718). `view_agent.go` 1841 → 1392 lines plus `view_agent_picker.go` (102), `view_agent_launch.go` (122), `view_agent_attached.go` (274), `view_agent_manage.go` (80).
- Held shared helpers (status formatting, executor labels, scroll/window math, manage-kind labels) in the parent files so the per-mode files only own their own renderer plus the helpers exclusively used by that mode (e.g. `attachedLayoutDims` / `renderAttachedRail` moved with `renderAgentAttached`).
- Build + `go test ./...` clean. Removed the corresponding "split the 1888-line update_agent.go / 1788-line view_agent.go by mode" entry from WORK.md polish.
- Also dropped the open `cleanup.go` scrubber decision from WORK.md — the file is now deleted per the git status, matching the existing "no one-user migration code" rule.

### 2026-04-22 — Agent manage page: fix edit entry, add delete/duplicate, iteration fields
- Fixed field editing being silently unreachable: `updateAgentManage` had `return m, m.beginAgentManageEdit()`, and because Go evaluates return operands left-to-right, the first slot snapshotted `m` before the pointer-receiver mutation set `agentManageEditing = true`. Split into `cmd := m.beginAgentManageEdit(); return m, cmd` so the mutation lands on the returned value.
- Restructured the Fields panel render in [view_agent.go](view_agent.go) `renderAgentManage`: when editing, the right pane shows `Editing: <label>` + the textarea (no longer appended after the full field list, where `capLines` could clip it on shorter terminals). When not editing, hint line now advertises `tab focus · enter edit · ctrl+s save · n new · D dup · d del`.
- Added delete (`d`, y/n confirm) and duplicate (`D`, `-copy-<hhmmss>` id) for presets and providers. Backed by new `cockpit.DeletePreset` / `cockpit.DeleteProvider` in [internal/cockpit/presets.go](internal/cockpit/presets.go) + [internal/cockpit/providers.go](internal/cockpit/providers.go). Confirm state lives on the model so the list and editor modes stay independent.
- Exposed iteration-policy fields for presets: `hooks.iteration.mode` (validated to `one_shot|loop_n|until_signal`), `hooks.iteration.n`, `hooks.iteration.signal`, `hooks.iteration.on_file`. Pressing enter on focus=0 now jumps focus to the fields pane instead of immediately opening an edit on the wrong field.

### 2026-04-22 — Agent Library framing + review-first new-job composer
- Reframed the old preset/provider manage page as an **Agent Library** in [view_agent.go](view_agent.go) / [update_agent.go](update_agent.go): user-facing language now treats presets as **recipes**, providers stay providers, and the page explicitly signals the bigger direction (`recipes + providers now; prompts/hooks/profiles move toward reusable components`).
- Reworked the manage surface so the selected item gets a real summary block (id/role/runtime/policies/hooks) and the editable fields are grouped into sections (`Identity`, `Prompting`, `Runtime`, `Hooks`, `Iteration`, `Policies`) instead of one long flat rail. This keeps recipe/provider editing compatible with the current JSON schema while aligning the UI with the larger object model.
- Added explicit scroll state for Agent Library list/detail panes plus paging keys (`pgup` / `pgdn`) so longer libraries and field groups remain navigable without relying only on cursor windowing. Mouse-wheel movement now also advances the corresponding library pane cursor/offset.
- Reworked the launch page from a bare preset/provider selector into a more composition-oriented **New Job** flow: tabs are now `Recipe`, `Provider`, `Brief`, and `Review`, and the review pane shows the assembled job (repo, selected sources, recipe/runtime/policy/hook summary, brief preview). The review pane is scrollable via wheel or `pgup` / `pgdn`.

### 2026-04-22 — Tmux statusline condense + repo-name window titles
- Tmux statusline was ~62 chars even when both providers were idle. Rewrote [internal/statusbar/tmux.go](internal/statusbar/tmux.go) `RenderTmuxLine` to abbreviate providers (`claude`→`cl`, `codex`→`cx`), drop the inner `·` between 5h and 7d, and only show the 5h reset time when `PctUsed ≥ 50` (i.e. when knowing "how long until reset" actually matters). Added `tmuxShortTime` for `3pm` on-the-hour / `3:04pm` otherwise. Line now renders as `cl 5h 42% 7d 12% · cx 5h 5% 7d 44%` (~34 chars).
- Tmux window titles now use the repo basename instead of `<id-tail>-<preset>`. Updated `windowName` in [internal/cockpit/runner_tmux.go](internal/cockpit/runner_tmux.go) to prefer `filepath.Base(j.Repo)` with the old preset/id fallback for freeform launches without a repo. Scales cleanly to 10+ parallel sessions because the title is just the repo, not the job-unique slug.

### 2026-04-22 — Cockpit jobs dashboard: stable columns, clearer filter, rewritten peek
- Jobs list columns now have fixed widths (`repo=12 · status=9 · preset=12`, narrower on small terminals) so repo/status/preset stay vertically aligned instead of drifting with the task column. Added a dim column header row above the list.
- Filter chips rework in [view_agent.go](view_agent.go) `renderAgentJobsHeader`: the active filter is a bracketed accent pill (`1 [all 5]`), inactive chips are separated by `·` bullets, zero-count chips fully dim. Much easier to spot what's filtered at a glance.
- `renderAgentPeek` rewritten: dropped the duplicated "Live Peek" inner header and the task-summary line that duplicated the left-panel task column. Metadata is now an aligned key/value block (`status / preset / executor / age / note / sources / brief`) capped by a single `── output ──` divider (or `── session log ─` for tmux-backed jobs). Wraps the same scrollable body as before.

### 2026-04-22 — Cockpit usage bar cleanup + tmux statusline fix
- Rewrote provider usage rendering in [view_agent.go](view_agent.go): dropped the `●/○` bar glyphs and split the single `renderProviderLimitsLine` into `renderProviderLimitsLines` → one row per provider (`claude` / `codex`) with threshold-colored `%`. Kept the 5h reset time, dropped the noisy 7d reset.
- Fixed the tmux statusline "gobbledegook": tmux's `#(...)` only interprets its own `#[fg=…]` markup, but `~/.codex/statusline.sh` emits ANSI escapes, so the bar showed literal `^[[38;2;…` garbage. Replaced the tmux integration with a new `sb tmux-status` subcommand (top of [main.go](main.go)) that reuses the `statusbar` package and emits tmux-native markup — see [internal/statusbar/tmux.go](internal/statusbar/tmux.go).
- Wired [internal/cockpit/tmux.go](internal/cockpit/tmux.go) `status-right` to call `sb tmux-status` via `os.Executable()` for a bulletproof absolute path, leaving the user's personal `~/.codex`/`~/.claude` scripts untouched. Tmux line now shows `claude 5h 42% @3:00pm · 7d 12%  ·  codex 5h 5% @7:19pm · 7d 44%`.

### 2026-04-21 — Agent cockpit UX fixes (typing, selection, delete)
- Fixed list cursor/selection drift: `renderAgentList` grouped jobs (needs-attn / running / recent) while `updateAgentList` indexed into the ungrouped slice, so enter/a/s/r/d acted on the wrong job. Added shared `orderAgentJobs` used by both sides.
- Rewrote attached-view key handling as a two-mode focus model: transcript focus (shortcuts + j/k scroll, default after attach) vs input focus (all letters type freely, only `alt+enter`/`esc`/`tab` intercepted). `tab` or `i` switches to input; `esc`/`tab` leaves it. Fixes "can't type a/s/r/j/k into chat".
- Added job delete path end-to-end: `Registry.Delete`, `Manager.DeleteJob` (stops PTY first), `MethodDeleteJob` protocol + dispatch, `SocketClient.DeleteJob`. UI key `d` (prompts y/n in list, immediate when attached).
- Attached view shows preset · status badge · executor · id · age · exit code · note · sources preview, plus a focus indicator line. Input panel hides when the job isn't running, with an explicit "no input accepted" hint.
- Footer + help overlay updated to match. Agent list now clamps cursor when jobs shrink and adds `g/G` jump.

### 2026-04-21 — Agent cockpit launch/runtime/layout fixes
- Fixed freeform launch repo fallback: `N` now defaults to the current project repo (or process cwd), and the manager also falls back to `os.Getwd()` instead of hard-failing empty-repo launches.
- Fixed Codex preset wiring: seed presets no longer inject a duplicate `exec` subcommand, so Codex jobs build as `codex exec -` instead of `codex exec exec -`.
- Tightened attached-view sizing so header/meta/input stay visible on shorter terminals; transcript height is now derived from the actual rendered chrome instead of a brittle fixed subtraction.
- Added cockpit tests covering Codex command construction and repo fallback for freeform launches.
- Hardened agent actions so `approve` and `delete` both require explicit `y/n` confirmation before changing job state or deleting source lines.
- Switched Codex jobs from fake history replay to native session resume: first turn captures `thread_id` from `codex exec --json`, follow-ups use `codex exec resume <thread_id> ...`.
- Normalized legacy Claude/Codex executor args in runtime so older presets that still include `--print`, `exec`, or `--json` don't break multi-turn command construction.
- Simplified cockpit flow/UI: launches auto-attach into chat, idle jobs open input-focused, attached chat uses `enter` to send, and the jobs page now shows a side detail pane instead of a flat list only.
- Replaced the attached raw transcript feel with structured chat rendering from job turns plus live in-flight output, added optimistic local echo for sent user messages, and stopped forced auto-scroll while reading older messages.
- Bounded the agent dashboard panes to a shared max height and windowed the jobs list so the selected-job detail pane no longer overruns the page.
- Fixed startup viewport bleed-through so background WORK.md discovery no longer injects dashboard content into the Agent page when you switch there immediately after launch.
- Split attach focus behavior: freshly launched chats can open ready to type, but attaching to an existing job now starts in transcript focus so scrolling doesn't fight the input box.
- Switched attached chat rendering to a local turn snapshot + pending/live state model so sent messages can appear immediately and viewport updates don't depend on a fresh daemon fetch every time.
- Optimistically append the just-sent user turn into the attached chat state immediately on send success so the message is visible before daemon/job refresh catches up.
- Added a real management path for presets/providers from the Agent page: create preset/provider templates and jump straight into editing hooks/executor config.
- Filtered legacy `claude-interactive` / `codex-interactive` provider files out of the UI so old seeded duplicates stop overlapping with the single current Claude/Codex providers.
- Added startup cleanup that rewrites old preset/provider JSON into the current shape: removes `*-interactive` provider files and strips redundant Claude/Codex executor args from on-disk config.

### 2026-04-21 — Agent cockpit V0.5 landed (foreman daemon + split presets/providers)
- `cmd/foreman` is now a real daemon: `ListenUnix` + `Serve` expose the Manager over an NDJSON unix socket at `~/.local/state/sb/foreman.sock`
- New `internal/cockpit/` files: `protocol.go` (Envelope / Method / payload types), `server.go` (accept loop, per-conn writer goroutine, lazy event subscription), `client.go` (SocketClient + `EnsureDaemon` auto-spawn that dials then re-dials after start), `iface.go` (small `Client` interface Manager and SocketClient both satisfy), `detach_unix.go` (setsid so foreman outlives sb)
- sb now dials the daemon by default; falls back to in-proc Manager if foreman isn't on PATH. `cockpit_daemon` / `cockpit_foreman_bin` config fields control this; header badge shows `[daemon]` vs `[in-proc]`
- Presets split from providers: presets carry a *suggested* executor but any `ProviderProfile` can override at launch time. Seed presets grew 4 → 17 (role-centric: senior-dev, bug-fixer, test-writer, refactor, code-analyzer, explainer, pm, docs-writer, scaffold, rfc, docs-tidy, classify, summarize, shell-test, shell-lint, shell-build, shell-escape)
- New `~/.config/sb/providers/*.json` seed dir with 8 profiles: claude, claude-interactive, codex, codex-interactive, ollama-qwen, ollama-llama, ollama-gemma, shell
- Launch modal gains provider picker; `tab` now cycles preset → provider → brief
- New `socket_test.go` drives a server+client pair end-to-end (launch shell job, observe stdout/status events, read transcript)
- Migration note: existing users with `~/.config/sb/presets/` populated won't reseed; delete the dir to pick up the role-centric IDs

### 2026-04-21 — Agent cockpit control-plane pass
- Tightened cockpit height management so the list/detail view and attached chat stay inside the app chrome on shorter terminals instead of growing too tall.
- Reworked the job list into a more operational cockpit: it now shows total/live/running/attention counts plus session usage grouped by provider/model, so Claude/Codex/Ollama distribution is visible at a glance.
- Reworked attached chat into a split cockpit with a persistent sessions rail and `[` / `]` quick-switching, which makes iterating across multiple live conversations much faster.
- Sending a follow-up turn now appends the user message into the local transcript immediately, so the chat shows what you sent before the provider reply comes back.
- `StopJob` now records explicit user-stop intent and returns the job to `idle` with note `stopped` instead of looking like a generic provider failure. Added a manager regression test for that path.


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

### 2026-04-28 — Agent list advance-state column + campaign control pass
- Added an explicit advance-state column to the Agent jobs list in [`internal/tui/view_agent.go`](internal/tui/view_agent.go), so queued/serial launches now scan as `solo`, `1/2 active`, `2/2 next`, `1/3 review`, etc instead of hiding that progress only in the detail pane.
- Expanded the selected-job detail pane to surface `advance`, `campaign`, `next up`, and a small `campaign controls` block for queued sequences, which makes `approve`, `skip`, and `retry` read as queue operations rather than generic delete semantics.
- Tightened the action-hint copy for queued and review states to say what happens to the queue (`approve and advance`, `skip item and promote next`, `retry current item`).
- Added focused regressions in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go) covering queued advance-state rendering and campaign-control / next-up visibility.

### 2026-04-28 — First-class skip job path
- Added a real `SkipJob` control-plane method across [`internal/cockpit/iface.go`](internal/cockpit/iface.go), [`protocol.go`](internal/cockpit/protocol.go), [`client.go`](internal/cockpit/client.go), [`server.go`](internal/cockpit/server.go), and [`manager.go`](internal/cockpit/manager.go) so `K` no longer piggybacks on `DeleteJob`.
- `SkipJob` now preserves the job record, marks it `StatusCompleted` with `SyncBackSkipped` + note `skipped by operator`, emits a status event, and then promotes the next queued job via the normal foreman path.
- Updated the TUI in [`internal/tui/update_agent.go`](internal/tui/update_agent.go) and [`internal/tui/view_agent.go`](internal/tui/view_agent.go) so skip prompts explicitly say they keep history, and skipped items render as `skipped` rather than disappearing or reading like generic `done`.
- Added regression coverage in [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go) for skip-preserves-record / next-queued-job-starts, plus the matching TUI test-double update in [`internal/tui/view_agent_test.go`](internal/tui/view_agent_test.go).

### 2026-04-28 — Campaign-tail skip control
- Added a campaign-scope `SkipCampaign` path across the cockpit control plane (`iface` / `protocol` / `client` / `server` / `manager`) so the operator can skip the current item and the remaining queued tail in one explicit action instead of hammering `K` repeatedly.
- `SkipCampaign` preserves every affected job record, marks the current one `skipped by operator`, marks later queued siblings `skipped by campaign abort`, and leaves them as `StatusCompleted` + `SyncBackSkipped` so the queue history stays visible.
- Wired the TUI to `C` in both list and attached/review states via [`internal/tui/update_agent.go`](internal/tui/update_agent.go) and [`internal/tui/update_agent_attached.go`](internal/tui/update_agent_attached.go), then updated [`internal/tui/view_agent.go`](internal/tui/view_agent.go) and [`internal/tui/view.go`](internal/tui/view.go) so the action hints, campaign-controls block, and help overlay differentiate item skip (`K`) from campaign-tail skip (`C`).
- Added a manager regression in [`internal/cockpit/manager_test.go`](internal/cockpit/manager_test.go) covering "skip current + queued tail" behavior, including the preserved notes on current vs later campaign items.

### 2026-04-28 — Extracted Bubble Tea app into internal/tui
- Moved the root TUI implementation files (`model.go`, `styles.go`, `update*.go`, `view*.go`, plus focused tests) into the new [`internal/tui/`](internal/tui/) package and renamed them from `package main` to `package tui`.
- Added [`internal/tui/run.go`](internal/tui/run.go) as the TUI entry point. It now owns config bootstrap, tmux bootstrap, preset/provider seed/load, daemon manager wiring, and the Bubble Tea program lifecycle.
- Reduced [`main.go`](main.go) to CLI dispatch only: `sb tmux-status` still renders the tmux status line directly, and the normal app path now delegates to `tui.Run()`.
- Why: this clears the active refactor blocker around keeping the cockpit/operator loop work moving while preventing the root package from becoming the dumping ground for every TUI concern.

### 2026-04-27 — Pre-approve post-hook execution preview
- Closed the V1 approve-time review gap: the review pane now dry-runs each post-hook before approval and shows ✓/✗/⊘ instead of only listing pending hook labels and discovering failures after approve.
- Added `HookPreview` + `HookPreviewStatus` types and `PreviewCmd`/`PreviewSafe` fields on `ShellHook` in [internal/cockpit/types.go](internal/cockpit/types.go).
- Added `RefreshPostHookPreview` / `LoadPostHookPreviews` / `PreviewPostHooks` in [internal/cockpit/review.go](internal/cockpit/review.go), backed by a per-job `<artifacts>/post_hook_preview.json` cache with a 5-minute TTL. Dry-runs are 10s-bounded, prepended with `GIT_PAGER=cat NO_COLOR=1`, and refused (`HookPreviewSkipped`) when the effective cmd contains shell redirection or a known mutating verb (`git push`, `npm install`, `rm -`, `sed -i`, etc). Authors can opt out per-hook via `preview_safe: true` and supply a side-effect-free `preview_cmd` for cases where the production cmd is mutating but a safe sibling exists.
- Rendered the preview block in [view_agent.go](view_agent.go) `renderReviewLines` under the existing changed-files / hooks blocks. Each row shows glyph + name + duration (ok), exit code (would_fail), or skip reason. When any preview is `would_fail`, the approve hint flips to a warn-styled "post-hook would fail — review output before pressing a" and the row in the jobs list picks up a `· !hook` marker (driven only by cached previews so the list stays free of synchronous work).
- Added regressions in [internal/cockpit/review_test.go](internal/cockpit/review_test.go) covering ok/would_fail status assignment, mutating-cmd skip + `PreviewCmd` override, and the cache-TTL fast path.

### 2026-04-27 — Compact history replay for ollama follow-ups (token discipline)
- Fixed the last real replay-only token leak in [internal/cockpit/manager.go](internal/cockpit/manager.go) `renderHistoryReplay`: when a job is on turn 2+, the replay now substitutes turn 0's full composed launch prompt (system prompt + before-hooks + sources + freeform + after-hooks) with the compact `Task` summary. The first turn still ships the full prompt so the model sees its persona and source context once; subsequent turns skip re-shipping it because the assistant's reply has already absorbed that framing.
- Codex is unaffected (uses native `thread_id` resume from 2026-04-21) and claude is unaffected (uses `--resume <session_id>`); ollama and any future replay-only providers are the load-bearing case.
- Audited seed presets in [internal/cockpit/presets.go](internal/cockpit/presets.go) and the user's `~/.config/sb/presets/*.json`: all system prompts are 1-3 sentence role descriptions, no chatty `### Style` / `### House Rules` prompt hooks, so an `Unattended` filter on `PromptHook` would be speculative API surface and is deferred until real bloat appears.
- Added regression in [internal/cockpit/manager_test.go](internal/cockpit/manager_test.go) `TestRenderHistoryReplayCompactsLaunchPromptOnFollowUps` covering the full-prompt-on-turn-1 vs compact-task-on-turn-2+ split.

### 2026-04-27 — Help overlay column wrapping cleanup
- Reworked [view.go](view.go) help rendering so the shortcut key column stays fixed while the description wraps only within the right-hand column. That lets the help overlay stay readable on narrower terminals without forcing the dialog width out to 120 columns.
- Tightened the help overlay width clamp to a more reasonable responsive cap and made scrolling operate on the fully wrapped visual lines, so long Agent help entries no longer break the layout or scroll awkwardly.

### 2026-04-27 — Shared dashboard Ctrl+C / target-name fix
- Fixed cockpit dashboard targeting in [internal/cockpit/bootstrap.go](internal/cockpit/bootstrap.go) and [internal/cockpit/tmux.go](internal/cockpit/tmux.go) so the shared `sb` window is now targeted by its fixed `main` name instead of assuming tmux window index `0`, which avoids the `can't find window 0` path after window churn or repair.
- Changed the root `Ctrl+C` behavior on that shared dashboard window: when more than one tmux client is attached, `Ctrl+C` now detaches only the current client instead of forwarding `SIGINT` into the single shared `sb` pane and killing the dashboard for every attached terminal.
- Updated tmux regressions in [internal/cockpit/tmux_test.go](internal/cockpit/tmux_test.go) plus the user-facing tmux notes in [README.md](README.md), and recorded the extra multi-client smoke-test follow-up in [WORK.md](WORK.md).

### 2026-04-27 — tmux status polling fix
- Tightened tmux status polling behavior without creating extra status-command churn: [internal/cockpit/tmux.go](internal/cockpit/tmux.go) keeps the tmux status refresh at 15 seconds, while [internal/statusbar/claude.go](internal/statusbar/claude.go) now caches Claude usage snapshots on disk for 5 minutes because tmux invokes `sb tmux-status` in a fresh process on each refresh.
- Updated the tmux regression in [internal/cockpit/tmux_test.go](internal/cockpit/tmux_test.go) and the tmux status-bar note in [README.md](README.md) to match the final polling/caching behavior.

### 2026-04-27 — Dashboard paging keybind pass
- Tightened dashboard navigation in [update.go](update.go) so `pgup` / `pgdn` now page through the project list, `home` / `end` jump to the first/last project, and preview full-page scrolling moved to `ctrl+b` / `ctrl+f` instead of overloading the list paging keys.
- Updated the in-app help in [view.go](view.go) plus the user-facing keybind notes in [README.md](README.md) so the dashboard now documents the clearer split between list navigation and preview scrolling.

### 2026-04-27 — Agent input/help and list-row guardrails
- Fixed the top-level key routing in [update.go](update.go) so `?` no longer opens the global help overlay while the Agent launch brief textarea is focused. That keeps in-progress new-job prompts intact instead of blowing the composer away mid-typing.
- Tightened Agent list summaries in [view_agent.go](view_agent.go) to collapse multiline task/brief/source text into one compact line before truncation, which stops long tasks from wrapping and breaking the jobs table layout.
- Added regressions in [update_agent_test.go](update_agent_test.go) covering the preserved launch-composer state and the new single-line task summary behavior.

### 2026-04-27 — tmux dashboard bootstrap reliability
- Fixed the cockpit tmux bootstrap in [internal/cockpit/bootstrap.go](internal/cockpit/bootstrap.go) and [internal/cockpit/tmux.go](internal/cockpit/tmux.go) so `sb` now treats a missing `tmux -L sb` socket as "no session yet" instead of a fatal error, which stops clean first launches from falling into `[exec-fallback]`.
- Reworked dashboard startup/repair so the shared `sb-cockpit` session now creates `main` with the real `sb` process on first launch and respawns window `0` when it finds a stale shell or broken dashboard pane instead of blindly trying to create another window at an occupied index.
- Added tmux regressions in [internal/cockpit/tmux_test.go](internal/cockpit/tmux_test.go) covering the missing-socket case plus dashboard repair/no-op behavior, so relaunching `sb` while another client is already attached stays on the intended shared-session path.

### 2026-04-27 — Foreman review bundle + task/prompt split
- Split operator-facing task text from executor launch context in [internal/cockpit/types.go](internal/cockpit/types.go), [internal/cockpit/hooks.go](internal/cockpit/hooks.go), and [internal/cockpit/manager.go](internal/cockpit/manager.go): jobs now persist a concise `task`/`brief` for the cockpit while keeping the full composed launch prompt separately for the actual first turn. That stops the queue/list/detail views from filling up with system prompt + hook sludge while preserving runtime behavior.
- Added review-artifact capture in [internal/cockpit/review.go](internal/cockpit/review.go) and wired it from the exec/tmux finish paths in [internal/cockpit/manager.go](internal/cockpit/manager.go) and [internal/cockpit/runner_tmux.go](internal/cockpit/runner_tmux.go). Review snapshots now persist changed files, git diff stat, hook activity from the event log, and pending post-hook labels under each job's artifacts dir.
- Started actually using `events.jsonl` by persisting cockpit events from [internal/cockpit/manager.go](internal/cockpit/manager.go), which makes hook/review history available after daemon restarts instead of existing only as live fanout.
- Reworked the Agent right pane in [view_agent.go](view_agent.go) so the same review block now shows sync-back preview, changed files, diff stat, hook activity, and queued campaign `next up` context. That makes approve/review materially closer to a PR-style operator pass instead of just “session text plus task deletion preview”.
- Fixed retry context in [internal/cockpit/manager.go](internal/cockpit/manager.go) so relaunches preserve the original freeform text instead of dropping that extra instruction on retry.
- Tightened sync-back safety in [internal/cockpit/review.go](internal/cockpit/review.go) / [internal/cockpit/manager.go](internal/cockpit/manager.go): the cockpit now snapshots repo status at launch, scopes review changed-files against that baseline, surfaces preexisting dirty files separately, and refuses approval when the source `WORK.md` / `DEVLOG.md` targets already have uncommitted changes.
- Smoothed the operator loop in [update_agent.go](update_agent.go), [view_agent.go](view_agent.go), and [view.go](view.go): the New Job composer now has an explicit repo tab (`recipe -> provider -> repo -> brief -> review`) so freeform launches can deliberately target a repo, and the jobs/attached views now expose explicit `R` resume behavior for waiting/stopped sessions instead of expecting the user to infer that they should just start typing.
- Added regressions in [internal/cockpit/manager_test.go](internal/cockpit/manager_test.go), [internal/cockpit/review_test.go](internal/cockpit/review_test.go), and the existing Agent rendering tests so the task/prompt split and review artifact path stay covered.

### 2026-04-27 — Cockpit now-list reliability pass
- Fixed the biggest cockpit lifecycle breakages across [internal/cockpit/bootstrap.go](internal/cockpit/bootstrap.go), [internal/cockpit/server.go](internal/cockpit/server.go), and [cmd/foreman/main.go](cmd/foreman/main.go): a second `sb` client no longer blindly kills dashboard window `0` or steals the unix socket, and the foreman now takes a lock before serving so duplicate daemons do not race each other.
- Hardened exec-job shutdown in [internal/cockpit/manager.go](internal/cockpit/manager.go): delete now waits for the canceled turn goroutine to finish before removing the registry entry, which closes the old race between `DeleteJob` and `runTurn`.
- Fixed freeform task labeling by persisting raw freeform text on [internal/cockpit/types.go](internal/cockpit/types.go) / [internal/cockpit/manager.go](internal/cockpit/manager.go) and rendering that in [view_agent.go](view_agent.go) instead of the fully composed prompt with hooks/system text.
- Restored cockpit approve flow in [update_agent.go](update_agent.go) / [view.go](view.go): `a` now arms approve confirmation from list or attached review, `s` explicitly advertises interrupt-not-kill semantics, and delete confirmation now warns when it will interrupt a running job first.
- Tightened Codex command construction in [internal/cockpit/manager.go](internal/cockpit/runner_tmux.go): resume no longer wedges arbitrary extra args between `--json`, `thread_id`, and prompt, and unsupported positional extras now fail fast instead of silently building brittle commands.
- Fixed cleanup safety for non-canonical sections in [internal/llm/llm.go](internal/llm/llm.go) by reinjecting missing non-canonical `##` blocks after the LLM pass, not just missing `- ` bullets. Added regressions in [internal/llm/llm_test.go](internal/llm/llm_test.go), [internal/cockpit/manager_test.go](internal/cockpit/manager_test.go), and [view_agent_test.go](view_agent_test.go).
- Added previewable sync-back in [internal/cockpit/syncback.go](internal/cockpit/syncback.go) and surfaced it in [view_agent.go](view_agent.go) / [update_agent.go](update_agent.go): review panes now show the actual task removals and `DEVLOG.md` additions that approval will apply before you press `a`.
- Added the first real foreman queue primitive in [internal/cockpit/types.go](internal/cockpit/types.go), [internal/cockpit/campaigns.go](internal/cockpit/campaigns.go), and [internal/cockpit/manager.go](internal/cockpit/manager.go): recipes now carry `launch_mode`, multi-task `task_queue_sequence` launches create one queued job per task inside a campaign, and the daemon promotes the next queued job for a repo after the current one is approved or deleted.
- Tightened operator flow in [update.go](update.go), [update_agent.go](update_agent.go), and [view.go](view.go): dashboard `A` now jumps directly into the current project's Agent picker, and Agent mode gained explicit `r` retry plus `K` skip controls so queued work is managed as a queue rather than as a side effect of delete.

### 2026-04-24 — Dead code sweep
- Removed unused functions surfaced by `staticcheck ./...`: `openEventLog` ([internal/cockpit/manager.go](internal/cockpit/manager.go)), `scrollSummary` ([update.go](update.go)), `countAssistantTurnsForView` and `minInt` ([view_agent.go](view_agent.go)).
- Dropped unused model fields `editSection`, `dashScroll`, `dashRightScroll` from [model.go](model.go).
- Cleaned dead initial assignments by switching to `var` declarations in `renderAgentManage` (title), the attached-chat input-label switch ([view_agent.go](view_agent.go)), and the project naming loop ([internal/workmd/workmd.go](internal/workmd/workmd.go)).
- Removed the empty `internal/ollama/` directory left over from the LLM-package refactor.

### 2026-04-22 — Agent short-terminal layout clamp
- Fixed Agent-page short-terminal overflow in [view_agent.go](view_agent.go) and [update_agent.go](update_agent.go): list/manage panels no longer force impossible minimum heights, picker/launch visible-row calculations no longer overrun tiny terminals, and attached chat now sizes its rail/chat panes against actual available rows.
- Tightened attached-chat rendering so header/footer rows are truncated to the real chat width and the viewport is capped to the remaining visible lines instead of letting panel chrome spill below the app footer on smaller screens.
- Added regression coverage in [view_agent_test.go](view_agent_test.go) that reproduces the bug on a 12-line terminal for both Agent list and attached-chat views.

### 2026-04-22 — Agent right pane task line + cleaner tmux log text
- Reworked the Agent right-hand peek in [view_agent.go](view_agent.go) so it now shows the selected job's task explicitly instead of making operators infer it from source metadata or the list row.
- Added `jobTaskText` to collapse sourced task text into a single task line and fall back to the freeform brief when a job has no sourced items.
- Tightened tmux/session-log rendering so the attached/review pane shows only the cleaned session text: dropped the extra `tmux-backed session` / `log:` wrapper lines and added a small chrome filter that removes box-drawing-only lines after ANSI/control cleanup.
- Added regressions in [view_agent_test.go](view_agent_test.go) covering box-drawing cleanup, wrapper removal, and the new task field in the right pane.

### 2026-04-22 — Agent Library framing + review-first job composition
- Reframed the old preset/provider manage screen in [view_agent.go](view_agent.go) as an **Agent Library** instead of a generic settings page. Presets now render as **recipes** in the UI, providers remain runtime definitions, and the copy explicitly marks prompts/hooks/profiles as future reusable component types rather than shoving them into the same flat editor.
- Reworked the manage page structure in [view_agent.go](view_agent.go) / [update_agent.go](update_agent.go): selected recipe/provider now gets a summary block, editable fields are grouped into domain sections (`Identity`, `Prompting`, `Runtime`, `Hooks`, `Iteration`, `Policies`), and the view gained explicit scroll/paging state so long libraries and long field sets stay navigable.
- Reworked the launch surface in [view_agent.go](view_agent.go) / [update_agent.go](update_agent.go) into a more composition-oriented **New Job** flow. Tabs are now `Recipe`, `Provider`, `Brief`, and `Review`, and the review pane shows the assembled run (repo, selected sources, recipe/provider runtime, policies, hooks, brief preview) before launch.
- Added regression coverage in [update_agent_test.go](update_agent_test.go) and [view_agent_test.go](view_agent_test.go) for the new review/scroll state and the updated Agent Library/launch copy.

### 2026-04-22 — Clickable top nav tabs
- Made the top header nav in `sb` mouse-clickable: left-clicking `Dashboard`, `Dump`, or `Agent` now switches pages directly instead of the header being display-only.
- Reused a shared top-nav hit test and page-switch helper so mouse navigation lands in the same base tab states as keyboard navigation, without adding a separate state path.
- Added regression coverage in [update_agent_test.go](update_agent_test.go) for header hit detection and mouse-driven page switching.

### 2026-04-22 — Dashboard window title cleanup
- Renamed the cockpit dashboard window title from `sb` to `main`, and aligned the Bubble Tea terminal title with it so window 0 is easier to identify among repo and agent panes.
- Files touched: `model.go`, `internal/cockpit/tmux.go`, `internal/cockpit/bootstrap.go`.

### 2026-04-22 — Agent picker/list bug fixes
- Fixed a filtered-job cursor bug in [update_agent.go](update_agent.go): the Agent list view already clamped the cursor for rendering, but the key handler still indexed with the raw `agentCursor`, so `enter`/`i`/`s`/`d` could silently no-op after filters or attach flows left the cursor past the end of the filtered slice. The update path now clamps before dispatching actions.
- Reworked sourced launch entry in [update_agent.go](update_agent.go) so `n` always starts at picker step 1 instead of auto-jumping into the current project's tasks. Picker and launch state now reset explicitly on each new launch, and backing out of picker step 2 clears stale checkbox/file state instead of carrying it into the next run.
- Fixed mouse-wheel routing in [update_agent.go](update_agent.go) for Agent picker and launch modes. Wheel scrolling now moves through the file list, task list, preset list, or provider list based on the active subview instead of incorrectly mutating the jobs-list cursor.
- Added regression coverage in [update_agent_test.go](update_agent_test.go) for filtered-list cursor clamping, `n` starting at picker step 1, picker back-state clearing, and wheel behavior in picker/launch modes.

### 2026-04-22 — Session log sanitization fix
- Fixed the tmux-backed session rendering in [view_agent.go](view_agent.go) by preferring tmux `capture-pane` snapshots for live peek/review output instead of trusting raw `pipe-pane` logs from full-screen Claude/Codex sessions.
- Tightened that snapshot path to capture only the currently visible pane contents instead of the full tmux history, which removes most startup splash/state-bar debris from the in-app review surface.
- Kept sanitized log-file fallback for cases where the pane is already gone, and taught [internal/cockpit/runner_tmux.go](internal/cockpit/runner_tmux.go) to persist one last clean pane snapshot when a tmux job dies but the pane is still capturable.
- Normalization still strips ANSI escapes, drops stray control bytes, applies backspaces, and treats carriage returns as line redraws so fallback output is less misleading.
- Added regression coverage in [view_agent_test.go](view_agent_test.go) and [internal/cockpit/tmux_test.go](internal/cockpit/tmux_test.go) for the sanitizer and the new `capture-pane` path.

### 2026-04-22 — Tmux relaunch target fix
- Fixed cockpit bootstrap in [internal/cockpit/bootstrap.go](internal/cockpit/bootstrap.go) to recreate/attach the dashboard via the fixed target `sb-cockpit:0` instead of the window name `sb`.
- This avoids collisions with tmux job windows named from the repo basename (for example multiple `sb` windows inside the `sb` repo), which could make relaunch print `can't find window: sb` or push startup into the fallback path even though tmux was healthy.

### 2026-04-22 — Agent header filter + limits pass
- Reworked the Agent jobs header in [view_agent.go](view_agent.go) so the active filter is called out on its own line, the counts row reads cleanly at a glance, and the provider mix line now reads as `sessions` instead of the vaguer `usage`.
- Dropped the visible `1-5` filter affordance in [update_agent.go](update_agent.go), keeping filter cycling on `tab` and adding `f` as the explicit list-page control.
- Reformatted Claude/Codex limit rows into stable columns (`5h`, `7d`, `extra`) so reset times and percentages line up vertically instead of drifting based on label length.

### 2026-04-22 — Agent jobs list column alignment
- Tightened the Agent jobs pane in [view_agent.go](view_agent.go) so each row renders as padded repo/task/status/id columns instead of a freeform bullet string. That keeps statuses and ids from drifting horizontally as task text length changes.
- Shortened the status label inside the list row only (`waiting`, `review`) so the compact column layout still holds on narrower terminals while the full status wording remains available elsewhere in the cockpit.

### 2026-04-22 — Agent tab became the real live cockpit
- Reworked the Agent jobs page around live triage in [view_agent.go](view_agent.go): the left pane now reads as repo/task/status rows (`working`, `waiting for input`, `needs review`, `done`) instead of a time-heavy preset/status list, and live sessions sort ahead of review/done work.
- Replaced the old selected-job summary pane with a real live peek backed by transcript/log files. The right pane now tails the latest exec transcript or tmux log output with only a thin metadata header, so you can see what the selected agent is doing before attaching.
- Added periodic job refresh in [update.go](update.go) so the Agent tab keeps updating even when no useful event arrives, which fixes the stale-status feel that showed up in the earlier header-badge attempt.
- Simplified main cockpit controls in [view.go](view.go) and [update_agent.go](update_agent.go): removed `approve` / `retry` from the visible live flow and kept `stop` as the only intervention action.
- Changed tmux stop semantics in [internal/cockpit/manager.go](internal/cockpit/manager.go) and [internal/cockpit/runner_tmux.go](internal/cockpit/runner_tmux.go): `s` now sends an interrupt without killing the tmux window, flips the session back toward input-ready state, and only surfaces a failure if the session actually dies afterward.
- Added regression coverage in [view_agent_test.go](view_agent_test.go) for operator-status heuristics and cockpit ordering.

### 2026-04-22 — Jobs header restyle + scrollable job detail
- Collapsed `renderAgentUsageSummary` + `renderAgentFilterBar` + `renderAgentFilterChip` in [view_agent.go](view_agent.go) into a single `renderAgentJobsHeader` that iterates jobs once. Each filter chip now colours hotkey (dim), label (dim/bold-accent when active), and count (text / primary / warn / accent) as separate roles so the number-to-swap no longer blurs into the count.
- Redesigned `renderAgentJobDetail` around a shared `kv` helper: compact meta rows, consolidated "Next" action + session-state line, and single "Latest" preview. Removed the redundant "Conversation" section that duplicated the next-action hint.
- Added `agentDetailOffset` on the model plus a generic `scrollWindow` helper so `pgup` / `pgdown` scroll the right-pane job detail with `▲/▼ N more` markers; cursor/filter/wheel-nav changes reset the offset.

### 2026-04-22 — Wheel scrolling, long-file jumps, and in-app Agent settings
- Enabled Bubble Tea mouse support in [main.go](main.go) and routed wheel events in [update.go](update.go) / [update_agent.go](update_agent.go) so the dashboard preview, project view, help overlay, Agent jobs list, and attached transcript/log review respond to the mouse wheel.
- Tightened inline `.md` editing in [update.go](update.go) and [view.go](view.go): edit mode now documents and honors `ctrl+home` / `ctrl+end` as top/bottom-of-file jumps for long files.
- Smoothed sourced Agent launches in [update_agent.go](update_agent.go) and [view_agent.go](view_agent.go) by defaulting `n` to the current project first, adding `b` to jump back to the full file picker, and surfacing a better source summary in the launch screen.
- Added an in-app Agent settings surface in [update_agent.go](update_agent.go) and [view_agent.go](view_agent.go) for editing preset/provider fields and hook JSON directly from the TUI, with save-time validation and persistence back to the existing preset/provider JSON files.
- Added focused coverage in [update_agent_test.go](update_agent_test.go) for current-project picker bootstrapping and preset hook-field parsing/saving.

### 2026-04-22 — Cockpit defaults, tmux styling, and hint cleanup
- Cleaned up the Agent page so routine controls stay in the footer/help overlay instead of being repeated inside the list, detail pane, and attached views. The cockpit panels now bias toward status and state instead of inline mini-manuals.
- Added a Codex-first launch default: new sourced/freeform launches now prefer the `senior-dev` preset and the `codex` provider when those profiles are available, instead of always landing on the first alphabetically sorted preset/provider.
- Styled the isolated `sb-cockpit` tmux session directly in `internal/cockpit/tmux.go`: mouse on, larger scrollback, `vi` copy-mode keys, and a cleaner bottom status bar with clearer active-window styling. Because the cockpit uses its own `-L sb` server, these options do not affect the user's personal tmux config.
- Added no-prefix scroll bindings for the isolated cockpit tmux session: wheel-up or `PageUp` now enter tmux scrollback automatically for the active pane instead of requiring raw tmux copy-mode commands.
- Added a regression test in `internal/cockpit/tmux_test.go` to verify the cockpit tmux session config path actually emits the expected tmux option commands.

### 2026-04-22 — Tmux cockpit path made coherent
- Finished the first real tmux-backed Claude/Codex path instead of mixing it with the older embedded-chat model. `internal/cockpit/runner_tmux.go` now launches the native interactive CLIs directly in tmux windows, normalizes legacy Claude/Codex args for that mode, and lets windows close cleanly so job status can advance instead of hanging in `running`.
- Rewired the Agent page so tmux-backed jobs use `AttachTmux` from the job list and on launch, while Ollama/shell jobs continue using the existing attached exec-chat view. Updated cockpit title/detail/help text to make the split visible, including an `exec-fallback` badge when tmux bootstrap is unavailable.
- Tightened cockpit tests by waiting for launched jobs to settle before tempdir cleanup, adding tmux-command normalization coverage, and skipping the unix-socket roundtrip test in environments where unix sockets are blocked.
- Fixed two lifecycle holes in the tmux path: dashboard `q` now detaches the cockpit client instead of blindly quitting when running inside `sb-cockpit`, and registry rehydrate now preserves tmux-backed running jobs so the daemon can reconnect them to still-live tmux windows after a restart.
- Added a clearer operator surface on top of that lifecycle work: Agent list/detail/attached views now expose explicit detach (`x`), live tmux-window state, and a finished-session log/review pane for tmux-backed jobs instead of only supporting live attach.
- Hardened tmux startup for detached foreman usage: cockpit tmux commands now inject a default `TERM=xterm-256color` when the daemon was started without a terminal environment, which fixes launches that previously failed with `open terminal failed: not a terminal`. The Agent UI now also surfaces that real launch note instead of the vague `tmux window not recorded for this job`.
- Fixed the tmux session bootstrap primitive itself: `EnsureSession` no longer uses `new-session -A`, and instead does an explicit `has-session` check before detached session creation. That avoids tmux falling into an attach/reuse path when `sb-cockpit` already exists, which was another source of confusing non-terminal startup failures.
- Added more reliable "return to sb" tmux root bindings. `F1` is still bound to window 0, but the cockpit now also binds `Ctrl+g` and `F12` so VS Code / Cursor terminals that intercept function keys still have a clean way to return from an attached job without using `Ctrl+C` and accidentally interrupting the agent process.
- Tightened that further by making `Ctrl+C` context-sensitive at the tmux root table: on window 0 (`sb`) it still passes through normally, but from attached job windows it now returns to `sb` instead of sending SIGINT to the agent process.

### 2026-04-21 — Agent cockpit control-plane pass
- Reworked the Agent page toward a real multi-session cockpit instead of a single-chat view. The list now shows top-level operational counts plus session usage grouped by provider/model so it is easier to see where Claude/Codex/Ollama jobs are concentrated.
- Reworked attached chat into a split layout with a persistent sessions rail and `[` / `]` quick-switching, which makes moving between multiple active conversations much faster.
- Tightened cockpit height calculations so the list/detail and attached layouts stay inside the terminal chrome on shorter screens instead of growing too tall.
- Sending a follow-up message now appends the user turn into the local attached transcript immediately, so the UI shows what was sent before the provider reply round-trip completes.
- `internal/cockpit/manager.go` now tracks explicit stop intent, returning stopped jobs to `idle` with note `stopped` instead of surfacing them as ambiguous provider failures. Added a regression test in `internal/cockpit/manager_test.go`.
- Added more power-user list flow: filter buckets (`all/live/running/attention/done`), `tab`/`1-5` filter switching, `i` to attach directly into input-ready iteration, and attach/back behavior that preserves the current job instead of resetting the cursor.
- Fixed an attached-chat clipping bug caused by mismatched width/height math: transcript rendering now uses the actual attached-chat column width, attached resize events refresh the viewport, and the chat panel no longer line-caps content after the viewport has already handled scrolling.
- Fixed an attached-chat optimistic-echo race in `update_agent.go`: stale daemon snapshots from `GetJob()` could overwrite the just-sent local user turn during immediate event/status refreshes, making a message appear only after the next send. Attached sync now preserves the optimistic local suffix until server state catches up.
- Removed transient "message sent" status churn from successful attached sends, because it changed cockpit height at the exact moment the viewport auto-followed and could push the newest visible turn below the fold.
- Added an explicit attached-layout recalculation step before transcript refresh/auto-follow, so viewport sizing now uses the real rendered textarea height instead of stale assumptions about the input area.

### 2026-04-21 — Agent: pm
- better readme for sb, how to use brain dump / cleaning etc

### 2026-04-21 — Jobs-are-chats: per-turn exec, multi-turn sessions
- Removed the PTY-embedded executor path entirely (`internal/cockpit/pty.go` deleted, `creack/pty` dropped from go.mod). It corrupted the transcript with ANSI/alt-screen sequences and locked each job to a single turn.
- Reworked `Job` into a turn-based session: added `Turns []Turn`, `SessionID`, `StatusIdle`, and `TurnRole` so the same primitive covers sourced tasks *and* freeform chats — one list view, one attached view, one primitive.
- `Manager.SendInput`/`SendTurn` now spawns a fresh `exec.Cmd` per turn. Claude uses native `--session-id` / `--resume`; codex/ollama replay the history to stdin; shell runs one-shot. Job stays `StatusIdle` between turns instead of flipping straight to completed.
- Post-shell hooks moved from per-turn to approve-time so follow-up turns don't re-trigger them. `ApproveJob` is what ends a conversation.
- UI: attached view wraps in `panelStyle` chrome to match the rest of the app; `StatusIdle` is treated as input-ready; input label calls out "turn in flight — wait for reply" vs "(conversation ended — no more turns)".
- Added freeform-chat entry: `N` in the job list jumps straight to the launch modal with empty sources and the brief textarea focused.
- Removed the ad-hoc parallel chat system (types, protocol methods, client methods, server dispatch, model fields, modes) in favor of unified jobs. `claude-interactive` / `codex-interactive` seed providers dropped — all providers now drive the same per-turn loop.

### 2026-04-21 — Agent: bug-fixer
- quick notes integration

### 2026-04-21 — Agent cockpit UX fixes (typing, selection, delete)
- Fixed list cursor/selection drift: `renderAgentList` grouped jobs (needs-attn / running / recent) while `updateAgentList` indexed into the ungrouped slice, so enter/a/s/r/d acted on the wrong job. Added shared `orderAgentJobs` used by both sides.
- Rewrote attached-view key handling as a two-mode focus model: transcript focus (shortcuts + j/k scroll, default after attach) vs input focus (all letters type freely, only `alt+enter`/`esc`/`tab` intercepted). `tab` or `i` switches to input; `esc`/`tab` leaves it. Fixes "can't type a/s/r/j/k into chat".
- Added job delete path end-to-end: `Registry.Delete`, `Manager.DeleteJob` (stops PTY first), `MethodDeleteJob` protocol + dispatch, `SocketClient.DeleteJob`. UI key `d` (prompts y/n in list, immediate when attached).
- Attached view shows preset · status badge · executor · id · age · exit code · note · sources preview, plus a focus indicator line. Input panel hides when the job isn't running, with an explicit "no input accepted" hint.
- Footer + help overlay updated to match. Agent list now clamps cursor when jobs shrink and adds `g/G` jump.

### 2026-04-21 — Agent: bug-fixer
- sometimes i open sb and it spins forever/bugs

### 2026-04-21 — Agent cockpit launch/runtime/layout fixes
- Fixed freeform chat launches end-to-end: `N` now seeds the launch modal with the current project repo (falling back to cwd), and `Manager.LaunchJob` also defaults empty repos to `os.Getwd()` instead of rejecting the job outright.
- Fixed Codex-backed presets building invalid commands. The preset seed no longer includes a redundant `exec` arg, so runtime command assembly now produces `codex exec -` as intended.
- Reworked attached transcript sizing in `view_agent.go` to subtract actual header/input chrome rather than a fixed constant, which keeps the header, transcript, and input visible on shorter terminals.
- Added focused cockpit tests in `internal/cockpit/manager_test.go` for Codex turn command assembly and freeform repo fallback.
- Hardened cockpit actions so `a`/`d` now stage an explicit `y/n` confirmation before approve sync-back or job deletion, including from the attached transcript view.
- Replaced Codex follow-up history replay with native resume: first turn now runs `codex exec --json`, captures `thread.started.thread_id`, and subsequent turns use `codex exec resume --json <thread_id> <prompt>`.
- Cleaned up Claude/Codex executor arg handling: seed Claude presets no longer carry redundant `--print`, and runtime now strips legacy `--print` / `exec` / `resume` / `--json` args so existing preset files keep working.
- Smoothed cockpit UX: launch now auto-attaches into the new chat, idle live jobs open with input focus, attached chat sends on plain `enter`, and the list page now includes a selected-job detail pane with last-turn preview and actions.
- Reworked attached chat rendering: the viewport now renders from structured `Turns` plus live streamed assistant output instead of the raw transcript log, sent user messages echo immediately, and viewport updates only auto-follow when you're already at the bottom or actively typing.
- Tightened the agent dashboard layout: the jobs list and selected-job detail pane now share the same capped panel height, and the job list windows around the cursor instead of growing unbounded.
- Fixed shared-viewport startup bleed-through: `projectsLoadedMsg` now only repaints the markdown viewport when you're actually on Dashboard or Project, so opening Agent immediately after startup no longer shows the top discovered WORK.md content there.
- Split agent attach focus behavior: new launches can still open input-first, but attaching to an existing job now starts in transcript focus so scrolling and reading old messages don't get hijacked by the textarea.
- Reworked attached chat state management to keep a local snapshot of job turns plus pending user text and live assistant output, instead of rebuilding entirely from fresh `GetJob` calls on every refresh. This stabilizes immediate local echo and reduces scroll jitter.
- Tightened immediate local echo further by appending the just-sent user turn into the attached chat's local turn list as soon as `SendInput` succeeds, so the last message shows up immediately rather than waiting for daemon state to round-trip.
- Added minimal preset/provider management to the Agent page: `p` creates a preset JSON template (including hook examples) and opens it in the editor, `v` does the same for a provider, and `P` / `V` open the config directories directly.
- Filtered legacy provider IDs `claude-interactive` and `codex-interactive` out of `LoadProviders`, so users with older `~/.config/sb/providers/` seeds no longer see duplicate Claude/Codex entries in the cockpit.
- Added `CleanLegacyConfig` at startup to rewrite old cockpit config instead of papering over it at runtime: it deletes `claude-interactive.json` / `codex-interactive.json` and strips redundant Claude/Codex executor args from provider and preset JSON on disk.
# 2026-05-01

- tightened tmux session supervision and transcript readability for Agents/Foreman runs
- preserved meaningful indentation in sanitized session logs so review panes stop mangling commands/code blocks
- widened tmux pane fallback detection to treat common follow-up / confirmation phrases as operator-yield signals when a model forgets to print `SB_STATUS`
- added regression coverage for Foreman unattended launch flags and `ctrl+r` attended relaunch behavior
- deduplicated Claude/Codex launch-policy argv assembly so exec turns, tmux runs, Foreman queue starts, and attended takeovers all share the same permission/runtime flag builder
- broadened tmux fallback handoff detection so direct questions, soft keep-going offers, and GUI-style choice prompts all yield back to the human
- gated tmux supervisor yields behind a 10-second quiet period on the session log so handoff phrases only trigger once the turn has actually gone idle
- split Foreman human handoffs into a first-class `awaiting_human` state so yielded jobs release Foreman concurrency while still holding same-repo write locks until the operator resolves them
