# Multi-Account Overhaul

## Goal

Support multiple subscriptions for the same provider, especially Claude and Codex, without forcing the user to rebuild all of their setup per account.

The current desired outcome is narrower than a full account-management platform:

- keep shared provider setup shared
- swap only the login/auth identity
- let the user decide later whether switching should be global-only or available per session/job

## Reality Of The Upstream CLIs

Claude and Codex currently behave like single-live-login tools.

In practice:

- Claude keeps live auth in `~/.claude/.credentials.json` unless `CLAUDE_CONFIG_DIR` points elsewhere
- Codex keeps live auth in `~/.codex/auth.json` unless `CODEX_HOME` points elsewhere
- logging into another account usually replaces the currently live auth file

That means there is no true built-in "multiple saved accounts at once" model unless `sb` creates one on top.

## What This Means Product-Wise

There are two viable product directions.

### Option A: Global Login Swap

This is the simpler model.

- `sb` stores named saved auth snapshots for each provider
- one account is active per provider at a time
- switching restores that saved auth snapshot into the live Claude/Codex auth location
- all shared config, prompts, hooks, rules, history preferences, etc. stay shared

What first-time setup looks like:

1. User logs into account A normally.
2. `sb` saves the current live login as a named slot, e.g. `claude/work`.
3. User logs into account B normally.
4. `sb` saves that current live login as another slot, e.g. `claude/personal`.
5. From then on, `sb` can switch between saved slots by restoring the chosen auth blob.

Strengths:

- matches the real upstream CLI behavior
- smallest implementation
- easy to explain
- works both inside and outside `sb` if the live auth file is replaced on disk

Weaknesses:

- not simultaneous
- first-time setup is somewhat manual
- "multi-account" is really "save and restore the one live login"

### Option B: Per-Session Isolated Accounts

This is the more powerful model.

- each tmux job or CLI launch runs with its own account-specific environment
- Claude jobs can vary via `CLAUDE_CONFIG_DIR`
- Codex jobs can vary via `CODEX_HOME`
- two sessions could use two different accounts at the same time

Strengths:

- true simultaneous multi-account usage
- enables different jobs to run under different subscriptions
- closer to a real account-aware runtime

Weaknesses:

- more complex implementation
- requires account-specific runtime env injection everywhere Claude/Codex are launched
- raises new UX questions about defaults vs per-job overrides
- makes "shared setup, only login changes" harder unless shared and isolated files are carefully separated

## Recommended Order

If this is built, the recommended order is:

1. Build Option A first.
2. Keep the storage model compatible with Option B later.
3. Only add per-session account selection if the global-swap model proves too limiting.

That preserves the simplest UX now while leaving room for future tmux/job-level overrides.

## Suggested V1 Scope

If implementation starts later, the recommended V1 is:

- provider scope: Claude and Codex only
- public surface: small CLI only
- no large TUI management flow yet
- no browser-login automation
- no per-run overrides

Suggested commands:

- `sb account list`
- `sb account save claude <name>`
- `sb account save codex <name>`
- `sb account use claude <name>`
- `sb account use codex <name>`
- `sb account show`

This keeps the feature honest: `sb` is saving and restoring the current live login, not pretending the upstream tools already support native named accounts.

## Open Questions

- Should switching update the live auth files on disk globally, or only for `sb`-launched processes?
- Should account metadata live in `config.json` or in a separate account registry under `~/.config/sb`?
- Should the account label appear in the TUI header and provider-limit displays?
- When the active saved auth is expired, should `sb` report that as a disabled account state or just let the provider fail naturally?
- If per-session support is added later, should the default remain global with optional per-job override, or should every job explicitly choose an account?

## Current Recommendation

Do not implement the broad overhaul yet.

The likely right first implementation is a small, explicit "save current login" / "use saved login" feature. If later real-world use shows that simultaneous sessions on different subscriptions matter, extend that into per-session isolated account homes via `CLAUDE_CONFIG_DIR` and `CODEX_HOME`.
