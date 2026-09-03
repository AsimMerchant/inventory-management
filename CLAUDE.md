# Store Register — project state

Inventory register for a large gathering. Replaces a handwritten ledger book. Run by
non-technical staff on **one Windows 11 laptop**, no installer, no dependencies.

## Where things are

| What | Where |
|---|---|
| Approved design, with screen mockups | `design/store-register.html` |
| Build specs | `specs/*.spec.md` — `00-index.spec.md` first |
| Codex project instructions and agents | `AGENTS.md`, `.codex/agents/` |
| Original photos of the handwritten book | `IMG-20260831-WA0000.jpg`, `IMG-20260831-WA0001.jpg` — the blank column headings, no entries |

The checked-in design is the source of truth for screens and wording. Read it before changing
anything user-visible.

## Current state — 3 September 2026

Current work is merged to `master` at `48e6632`. The committed build contains the
protected finance area, scalable product pickers, the merged money/order entry and the
third acquisition choice (Rent, Purchase, or reusable custom Other). The schema-5
slice moves supplier/payee names into one shared public party list so the
unauthenticated inward desk and authenticated finance screens use the same suggestions.

Only party IDs, current/previous names and merge targets are public. Money, purpose,
payment mode, financial links, account identity/mobile, timestamps and audit provenance
remain encrypted. The schema-4 to schema-5 migration preserves existing finance IDs and
history. This slice has passed the full race suite, Playwright in normal and
JavaScript-disabled modes plus restart/raw-file checks, plain-language review, vet,
the Windows cross-build and a fresh independent release gate. The gate verdict is
`READY`; `HANDOFF.md` is the detailed current record.

Next planned work is the already agreed ledger simplification: the routine recording
screen will support Billed/Paid/Received together. Supplier running balances, supplier
challans and broad search remain separate pending work; the whole ledger is not being
collapsed into literally one form.

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
- **Ordinary inventory has no login.** The shift screen puts a name on desk entries;
  only the protected financial area uses individual mobile/password accounts.
- **Over-issue and over-return are refused.** Partial returns are first-class.
- **A short return requires a remark** plus "still expected back" or "won't come back".
  Nothing is ever written off automatically.
- **Ordinary inventory exposes no settlement, payment or money.** Those facts exist only
  inside the authenticated encrypted financial area.
- **The inventory desk stops when issued goods return to its pool.** Authorized financial
  users separately record supplier returns, sales and related money; ordinary inventory
  never infers those outcomes.
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

## Historical state through 2 September 2026

### Work begun 2 September 2026

The user commissioned a protected financial ledger on branch
`feature/financial-ledger`. This deliberately supersedes the historical prohibitions
on authentication, money and supplier-return workflows inside the new authorized
area; the ordinary inventory screens retain their existing no-login workflow and
must expose no financial data. Reviewed specifications 17–21 govern the new work. See the
active-work section of `HANDOFF.md` for the current requirements and verification
state — it carries the committed slice-by-slice build table, the traps this build has
already hit and the named gaps the next spec must close. (`.agent-handoff/latest.md`
mirrors it but is gitignored, so it must never be the only record of anything.)

**All five specs, 17 to 21, are built and green.** The encrypted vault with individual
accounts and sessions; orders with the shared party, purpose and payment-mode lists;
money in and out with audited corrections, voids and a filtered printable journal;
supplier returns and stock sales with a neutral public stock projection so ordinary
arithmetic is right after a restart with nobody logged in; and the protected interface
with its two-step destructive actions. Every screen has been through
`plain_language_reviewer`, and the whole scenario has been driven in a real browser
against a real binary, including a pass with JavaScript switched off.

**The first financial build used schema 3; the current work uses schema 5.**
The current build reads schemas 1 through 5, so upgrading is safe. Older builds do *not*
necessarily refuse newer files cleanly: they may treat the mismatch as damage, set
the real data aside as `store-register.json.corrupt-<timestamp>`, fall back to `.bak`
and show pre-upgrade data behind the ordinary damage banner. Same trap as the
`v1.0.1` to `v1.1.1` upgrade, same procedural fix, and it cannot be fixed in the new
version because the defect is in the released reader. Before the new `.exe` is used:
back up `store-register.json`, delete the old `.exe` from the laptop and the pen drive,
then put the new one there.

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
