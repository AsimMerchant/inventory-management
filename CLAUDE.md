# Store Register — project state

Inventory register for a large gathering. Replaces a handwritten ledger book. Run by
non-technical staff on **one Windows 11 laptop**, no installer, no dependencies.

## Where things are

| What | Where |
|---|---|
| Approved design, with screen mockups | https://claude.ai/code/artifact/bfcf8682-3b0b-4933-b2e9-a7348d482457 |
| Build specs | `specs/*.spec.md` — `00-index.spec.md` first |
| Agent definitions | `.claude/agents/` — four, all in use |
| Original photos of the handwritten book | `IMG-20260831-WA0000.jpg`, `IMG-20260831-WA0001.jpg` — the blank column headings, no entries |

The artifact is the source of truth for screens and wording. Read it before changing
anything user-visible.

## What it does

Three events, one record each, all linked:

1. **Stuff came in** — product, how many, date, rent or purchase, supplier, challan no., who received it.
2. **Someone is taking** — who took it (name, department, mobile), who handed it over, when.
3. **Someone is returning** — when, who brought it back, who took it back in.

Plus: a "who is at the desk" tap at shift start, and read-only views for stock, who is
holding what, what came in, and from which suppliers.

## Architecture

Single self-contained Go binary. Local HTTP server on `127.0.0.1`, UI in the default
browser, all state in one human-readable file beside the executable.

- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` **must always succeed** — verified
  working, produces a PE32+ exe from this Linux machine.
- Standard library only. No cgo, so no `mattn/go-sqlite3` and no native GUI toolkit.
- Bind `127.0.0.1` only, never `0.0.0.0` — avoids the Windows Defender Firewall prompt.
- Atomic writes: temp file, fsync, rename current to `.bak`, rename temp over. A crash
  leaves the old good file or the new good file, never a truncated one.
- Go 1.27 installed. Developed and tested on Linux; the `.exe` is the last step.

## Decisions the user made — do not re-open

- **Stock is pooled per product**, not tracked per inward batch. A chair is a chair.
- **Product names are picked, never free-typed.** Creating a new product is a separate
  deliberate action. A split product silently halves the on-hand count.
- **People records may be sloppy; product records may not.** Two spellings of one man
  are two rows and nobody is nagged. The asymmetry: a duplicate person is a cosmetic
  problem a human can see and fix; a duplicate product corrupts the number the whole
  register exists to produce.
- **A person is name + mobile together.** One search field accepts either. An exact
  name match shows the existing people with their mobiles to pick from — offered, never
  enforced.
- **No passwords, no authentication anywhere.** The shift screen puts a name on entries.
- **Over-issue and over-return are refused.** Partial returns are first-class.
- **A short return requires a remark** plus "still expected back" or "won't come back".
  Nothing is ever written off automatically.
- **No settlement, no payment, no money anywhere in the program.**
- **Nothing about returning goods to suppliers.** The user: *"we care about I got 500
  chairs, I issued 500, i want 500 at end of event, thats it — there are other people
  who handle this going back to supplier."* The Suppliers tab is a plain record of what
  came in from whom.
- **Entries can be corrected or deleted**, keeping a visible note of what changed and
  who changed it. Corrections that would make the register impossible are refused.
- **Code signing was considered and rejected** — costs money, and the SmartScreen
  warning is a one-time "Run anyway" on a pen-drive transfer.

## The rule the whole design rests on

> The person on duty never types a number from memory, and never types an identifier.

Every quantity and every person comes off a list the software puts on screen. If a
screen would make someone remember something, that is a design bug, not a wording bug.

## Working agreements with the user

- One round of blocking questions, then propose with stated assumptions. Repeated
  clarifying rounds read to him as going backwards.
- Review subagent output yourself and surface only what genuinely needs his judgement.
  He explicitly said he will not review if nothing is flagged.
- Use Read/Write/Edit for file work, Bash for commands.
- Keep messages short.

## State as of 1 September 2026

Specs 01–12 are implemented and released as `v1.0.0`. Spec 13, which lets one issue
name multiple people sharing one total quantity, was merged through PR #1 at `77a40f0`.
The selected release version for that enhancement is `v1.0.1`.

The current tree passes the feature tests, full race suite, store tests, vet, Windows
amd64 cross-compile, acceptance greps and the documented register coverage gate at
97.1% (minimum 95%). The spec 13 contributor reports a successful Windows workflow;
Codex independently cross-compiled the artifact but did not repeat that manual run.

The `plain_language_reviewer` checked 175 built strings and found only the known
no-supplier wording conflict documented in `HANDOFF.md`; the specs disagree, so no
code change was made. Native Linux binaries passed the original end-to-end smoke and a
spec 13 smoke that stored and found one joint issue while counting its quantity once.

See `HANDOFF.md` for the current verification commands, known spec-text defects and
the exact continuation order. Codex-native project instructions and agents now live in
`AGENTS.md` and `.codex/`.
