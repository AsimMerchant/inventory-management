# Store Register — project state

Inventory register for a large gathering. Replaces a handwritten ledger book. Run by
non-technical staff on **one Windows 11 laptop**, no installer, no dependencies.

## Where things are

| What | Where |
|---|---|
| Approved design, with screen mockups | https://claude.ai/code/artifact/bfcf8682-3b0b-4933-b2e9-a7348d482457 |
| Build specs | `specs/*.spec.md` — `00-index.spec.md` first |
| Agent definitions | `.claude/agents/` — four, all in use |
| Original photos of the handwritten book | `IMG-20260831-WA0000.jpg`, `IMG-20260831-WA0001.jpg` |

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

## State as of 31 August 2026

Design approved. **Twelve specs written, reviewed and complete** — `specs/00-index…11-corrections`.
A plain-language pass over 139 user-visible strings is applied throughout specs 02
and 04–10. Every open question is settled and recorded as normative in `00-index.spec.md`;
nothing is waiting on the user.

Spec 11 (corrections) is the newest and the riskiest — its guards stop a correction
leaving the register impossible, including the non-obvious one where deleting a return
drives on-hand negative. It has the heaviest test cases for that reason.

All twelve specs have now been through `plain-language-reviewer`. Nothing is in flight.

**Next:** `go-local-app-engineer` implements in the build order in `00-index.spec.md`
(spec 01 first — it holds the fixture every other spec's tests are written against).
Then `plain-language-reviewer` over the built strings, then `release-gate`, which
refuses to certify without running the tests and the Windows cross-compile itself. The
user gets a Linux build to use and complain about before anything becomes an `.exe`.

Nothing has been implemented yet. There is no Go code in this repo.

**Not done, worth offering:** this folder is not a git repo. The user was asked and had
not answered when the session was saved.
