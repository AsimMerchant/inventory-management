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

### Work begun 2 September 2026

The user commissioned a protected financial ledger on branch
`feature/financial-ledger`. This deliberately supersedes the historical prohibitions
on authentication, money and supplier-return workflows inside the new authorized
area; the ordinary inventory screens retain their existing no-login workflow and
must expose no financial data. Reviewed specifications 17–21 govern the new work. See the
active-work section of `HANDOFF.md` and `.agent-handoff/latest.md` for the current
requirements and verification state.

Specs 01–12 shipped as `v1.0.0`. Spec 13, one issue naming multiple joint recipients,
shipped as `v1.0.1` and went to the stakeholders. Their feedback became specs 14, 15 and
16, released together as **`v1.1.1`**: `Dashboard` and `Change person` on every screen
and a working product picker on the log; product rename and one-action cascading
deletion that tombstones the product and all its entries while `Who did what` keeps
everything; and an optional challan number on an issue that any part of can be typed to
find the goods when they come back.

That work is verified: 372 tests under `-race`, vet clean, PE32+ cross-compile, all 57
spec-named tests present, every acceptance grep silent, `internal/register` coverage
95.4% against a 95% minimum. Spec 16's browser scenario passed in full through a real
browser against a real binary, and again with JavaScript off. The `release_gate`
reproduced all of it independently and probed the dangerous paths itself — concurrent
over-issue, concurrent over-return, a delete racing fifteen issues, the schema-1 to
schema-2 migration against binaries built from both tags.

**The one thing that must happen before the new `.exe` is used:** back up
`store-register.json`, then delete the old `.exe` from the laptop and the pen drive.
`v1.0.1` cannot read a schema-2 file and does not refuse it cleanly — it falls back to
the pre-upgrade `.bak` and blames damage. This cannot be fixed in `v1.1.1`; the defect
is in the released reader. The user chose the procedural fix on 1 September 2026.
`HANDOFF.md` has the detail.

Ten wording findings against the new strings, the non-load-bearing save-time recheck
tests, and the unbounded quantity field are all open and deferred, not declined. See
`HANDOFF.md` for the continuation order. Codex-native instructions live in `AGENTS.md`
and `.codex/`.
