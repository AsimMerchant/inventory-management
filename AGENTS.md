# Store Register — Codex project instructions

This repository builds a single self-contained Windows inventory register for
non-technical staff at a busy gathering. Treat correctness and recoverability as
more important than cleverness.

## Read before working

Read these sources in order:

1. `HANDOFF.md` for the engineering state and verification commands.
2. `CLAUDE.md` for product decisions and working agreements. Its final historical
   status paragraph is stale; Git and `HANDOFF.md` describe the current build.
3. `design/store-register.html` for approved screens and wording.
4. `specs/00-index.spec.md`, followed by the numbered spec for the task.
5. `.agent-handoff/latest.md` when continuing an interrupted agent session, but
   verify every claim against the working tree because the packet may be stale.

Never silently deviate from a spec or reword an approved user-visible string. If a
spec is genuinely inconsistent, implement its Contract where safe and report the
conflict.

## Non-negotiable constraints

- Standard library only; no cgo or native GUI toolkit.
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .`
  must succeed.
- Bind only to `127.0.0.1`, never `0.0.0.0` or an unspecified interface.
- Keep all state in the human-readable file beside the executable.
- Preserve the atomic save sequence and `.bak` recovery. Treat data loss as the
  worst possible failure.
- `internal/web` must not implement stock arithmetic. `internal/register` and
  `internal/store` must not import `net/http`.
- Product names are selected, never free-typed. People may be duplicated.
- Refuse over-issue and over-return. Corrections must never leave impossible stock.
- No authentication, money, settlement, or supplier-return workflow.
- Never require the person at the desk to remember a quantity or identifier.

## Working discipline

- Before editing, state assumptions and a short plan with verifiable success
  criteria.
- Make the smallest change that satisfies the current spec. Do not refactor,
  reformat, or clean up unrelated code.
- Preserve all pre-existing dirty-worktree changes unless the user explicitly asks
  to discard them.
- Remove only imports, variables, or helpers made unused by your own change.
- For a bug, reproduce it in a test first. For a feature, implement the required
  spec tests and then make them pass.
- Do not trust summaries that say tests passed; run the relevant commands yourself.
- Keep user-facing updates short and surface only decisions that genuinely need the
  user's judgment.

After meaningful implementation work, run:

```text
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

Also run the acceptance and grep checks in the applicable spec. Clearly separate
what was verified on Linux from behavior that still needs a real Windows check.

## Project agents

Codex-native agents live in `.codex/agents/`:

- `spec_writer`: write or repair precise specs; never implement them.
- `go_local_app_engineer`: implement or fix Go code from an agreed spec.
- `plain_language_reviewer`: read-only review of user-visible UI wording.
- `release_gate`: independent final verification; defaults to `NEEDS WORK`.

For a multi-file implementation task, delegate implementation to
`go_local_app_engineer` when subagents are available. Do not run two editing agents
in the same worktree concurrently. After implementation, use
`plain_language_reviewer` if user-visible text changed. Use `release_gate` only when
the build is claimed complete, and do not replace its independent checks with the
implementer's report. The primary agent owns integration and reviews every agent's
result before reporting to the user.

