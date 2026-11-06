# AGENTS.md — dark factory

The human gives a goal once; the factory delivers a reviewed PR. Guardrails and these rules hold unless the human explicitly allows an exception.

## Roles (Anthropic only, newest generation; prices per MTok input/output, checked 2026-11-04 at platform.claude.com/docs/en/about-claude/pricing)

| Role | Model | Price | Effort | Why |
|---|---|---|---|---|
| Orchestrator/Planner | `claude-sonnet-5-5` | $2 / $10 | high | High reasoning, not the priciest. Opus 5.5 goes to the Executor, so it isn't reused |
| Executor | `claude-opus-5-5` | $4 / $20 | default | Best coding model that isn't the priciest |
| Auditor | `claude-fable-5-1` | $10 / $50 | high | Most capable model available |
| Tooler | `claude-haiku-5-5` | $0.10 / $0.50 (≤100k-token prompt); $0.50 / $2.50 (>100k) | default | Cheapest model |

- Price rank: Fable 5.1 > Opus 5.5 > Sonnet 5.5 > Haiku 5.5.
- Claude Mythos 5.1 also costs $10 / $50, but it has limited availability and Claude Code can't select it. It is therefore not an alternative, and so not a tie.
- Choosing models: ask the human only if two models tie for a role.
- Launch the Orchestrator with `claude --model claude-sonnet-5-5 --effort high`.
- Orchestrator: plans, splits the goal into tasks, dispatches, waits and integrates. It never writes code.
- Executor: writes the tests first, then the code, in its worktree, and runs `task gate`.
- Auditor: starts with fresh context. It gets the goal, AGENTS.md and the PR diff, plus read-only use of the PR's worktree so it can run `task gate` and its own probe inputs. It answers ACCEPT or REJECT with reasons and never edits, commits or fixes. REJECT if:
  - tests echo the code instead of asserting the goal's behavior;
  - anything is weakened or skipped;
  - a test is deleted;
  - a protected file changed without sign-off;
  - a dependency is added or changed without being flagged;
  - any rule here or in the goal is broken.
  It also tries to break the build with inputs the tests lack.
- Tooler: handles setup and CI by itself, with no Executor and no Auditor. Changes to protected files still need the human's sign-off.

## Repo and machine

- Repo: `github.com/bijaya777pokhrel-hue/dark-factory-lab` (public). "Automatically delete head branches" is on.
- Git ≥ 2.43 is required by `wt`. It is built from source in `~/.local/bin`, which `~/.zprofile` and `~/.zshrc` put first on PATH. `task` is at `~/go/bin`.
- Worktrees live in `.worktrees/` (git-ignored). This comes from a per-machine `wt` user config, not the repo. In `~/.config/worktrunk/config.toml`:
  ```
  [projects."github.com/bijaya777pokhrel-hue/dark-factory-lab"]
  worktree-path = ".worktrees/{{ branch | sanitize }}"
  ```

## Subagents (herdr)

- Each subagent gets only its task. It must never spawn subagents (`--disallowedTools Agent`).
- Independent tasks run in parallel, each in its own herdr pane. Start them, then block once with the shell's `wait`:
  ```
  P=$(herdr pane split --current --direction right --cwd <worktree> --no-focus | jq -r .result.pane.pane_id)
  herdr agent start <name> --kind claude --pane "$P" -- --model <role model> [--effort high] --disallowedTools Agent
  herdr agent prompt <name> "<task>" --wait --timeout 3600000 &   # one line per task
  wait
  ```
- Wait only with herdr's blocking `--wait`. Never poll or re-wait in a loop.
- If a wait times out, stalls or returns `blocked`, run `herdr agent read <name>`, then escalate to the human.

## Worktrees (`wt`) and git

- Never work in the primary checkout. Use worktrees, and only through `wt`, with no hooks.
- Create a PR branch, always from freshly fetched `main`:
  `git fetch origin main && wt switch --create <branch> --base origin/main --no-hooks --no-cd -y`
- Remove it after the human merges:
  `git fetch origin && wt remove <branch> --no-hooks --foreground -y`
  If wt reports "Branch unmerged", don't use `-D`. Stop and escalate.
- Push only the PR branch: `git push -u origin <branch>`. Never push to `main`.
- Do everything else on GitHub through `gh` (`gh pr create`, and so on). Only the human merges.

## Gate and PR

- `task gate` must pass locally before any PR is opened. It is the only gate task and also what CI's single `guardrails` job runs. It runs, in order: `go build`, `go vet`, golangci-lint, staticcheck, govulncheck, then race tests with a 90% total-coverage check. The three linters are pinned in the Taskfile and run with `go run <module>@<version>`, never installed.
- The PR body must contain:
  - the `task gate` result;
  - a per-file coverage table: the verbatim `go tool cover -func=cover.out` output (rows are `file:line func %`, plus the total) in a fenced block, with no custom tooling;
  - `## Dependencies`: every module added or changed, or "none". Anything missing from this list counts as unflagged;
  - `## Protected files`: each protected file changed, with the human's quoted sign-off, or "none".

## Go tests

- Tests come first and are committed before the code they test.
- Each source file `x.go` gets exactly one blackbox test file `x_test.go` (`package <pkg>_test`).
- The thin CLI `main` is no exception. Its test builds and runs the binary.
- Each exported function and method gets one table-driven test that calls `t.Parallel()`. It asserts the behavior the goal describes, not whatever the code happens to output.
- Keep the CLI `main` thin: it only wires things up, and the logic lives in packages.
- Total coverage must be at least 90%. Never weaken, skip or delete a test.

## Protected files (the human must sign off on changes)

Taskfile, lint config, CI, Dependabot, AGENTS.md (and its CLAUDE.md symlink), `spec/target.html`, repo settings and rulesets.

Exception: the human said "You may edit protected files until PR #1 merges." That covers PR #1 only; after it merges, this list applies in full.

## Escalation

- If the same failure happens after two rounds, stop and escalate to the human.
