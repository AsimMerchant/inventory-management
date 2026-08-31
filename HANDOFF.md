# Handoff

You are picking up a build in progress. This file tells you what exists, what is left,
how to check the work is sound, and which mistakes are easy to make here.

Read `CLAUDE.md` first — it holds the product decisions and the working agreements. This
file is the engineering state; that one is the why.

## What this is, in three lines

A single self-contained Windows `.exe` that replaces a handwritten inventory ledger at a
large gathering. It runs on **one Windows 11 laptop**, no installer and no runtime, as a
local HTTP server on `127.0.0.1` that serves its screens to the browser already on the
machine. All state is one human-readable JSON file beside the executable.

The people using it are not technical and have had no training. That constraint decides
more design questions than any other.

## Read in this order

1. `CLAUDE.md` — product decisions, constraints, working agreements.
2. `design/store-register.html` — the approved design, with screen mockups. **This is the
   source of truth for screens and wording.** Open it in a browser; it is a real page.
3. `specs/00-index.spec.md` — build order, binding conventions, every settled decision.
4. The numbered specs, `01` through `12`.

The specs are precise and have been reviewed twice: once for correctness, once by a
plain-language pass over every user-visible string. **Follow them exactly.** Where a spec
and your instinct disagree, the spec wins. Where a spec is genuinely wrong, implement the
Contract section and *report the defect* — every previous pass found real ones that way,
and that reporting has been more valuable than the code.

## State

| Specs | What | Status |
|---|---|---|
| 01–03 | Data model + fixture, atomic persistence, stock arithmetic | **Done, committed** (`7c5f9c5`) |
| 04–06 | Server, page shell, shift screen, product picker | **Done, committed** (`11bb2c7`) |
| 07–09 | Stuff came in, someone is taking, someone is returning | **Done, committed** (`07bdd66`) |
| 10 | The four read-only tabs | **Implemented and tested, uncommitted** — 22 dedicated web tests |
| 11 | Corrections and guarded deletion | **Implemented and tested, uncommitted** — 27 dedicated web tests |
| 12 | Derived activity log and filters | **Implemented and tested, uncommitted** — 18 register tests and 21 web tests |

The spec 10–12 work is a large dirty-tree continuation of the interrupted Claude pass.
Do not discard or reset it. Codex completed the missing required test suites and reviewed
each subagent slice independently. As of the latest checkpoint,
`go test ./... -race -count=1`, `go vet ./...`, the Windows amd64 cross-compile and the
applicable acceptance greps all pass.

The read-only `plain_language_reviewer` checked 175 built strings and found only the
known no-supplier wording conflict recorded below; no code change is warranted while
the two specs disagree. A native Linux binary also passed the end-to-end smoke
scenario, including persisted JSON and the read-only log; the Windows amd64 artifact
was cross-compiled but still needs a real Windows runtime check. The independent
`release_gate` is the remaining step. Do not call the build ready before that gate.

Known spec-text defects found while implementing the required tests:

- Spec 11 shortens the fixture's full return remark in two examples. The implementation
  preserves the complete stored remark.
- Spec 12 says deleting `INW-0002` from T1 leaves 390 chairs received, but live
  `INW-0007` contributes another 500; the correct total is 890.
- Specs 11 and 12 prescribe different no-supplier wording for the shared `entryName`
  helper. The implementation conservatively preserves spec 11's existing-screen wording.
- Several spec 12 required-test paragraphs retain older wording that contradicts the
  later normative Contract. Tests and implementation follow the Contract.
- Spec 00's original forbidden-word greps scanned `design/store-register.html` and
  test names, so they failed on historical, non-shipped prose and tests that prove the
  forbidden UI is absent. The verification commands now scan shipped code only and
  exclude `*_test.go`.

## How to check the work

These are not optional and they are how every pass has been verified:

```
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .   # note the '.', not './...'
```

Plus the whole-build block at the foot of `specs/00-index.spec.md`, which greps for the
things that must never appear: `0.0.0.0`, money vocabulary, authentication vocabulary,
and any third-party dependency.

**Do not trust a report that the tests pass. Run them.** Two of the verification commands
in the specs were themselves broken and could never have passed as written; they were only
found because someone ran them.

## Running it for real

Static tests are not enough for this program. Build it into an empty directory and drive
it, the way each pass has been checked:

```
mkdir /tmp/e2e && go build -o /tmp/e2e/register . && cd /tmp/e2e && ./register &
curl -s http://127.0.0.1:8765/shift
```

**The trap:** the program hunts ports 8765→8785 when one is busy. A leftover process from
an earlier test keeps 8765, your new binary silently takes 8766, and you spend twenty
minutes debugging a stale server. Kill by PID from the socket table before each run:

```
for p in 8765 8766 8767; do
  pid=$(ss -ltnp | grep ":$p " | grep -oP 'pid=\K[0-9]+' | head -1)
  [ -n "$pid" ] && kill -9 $pid
done
```

A full working scenario, useful as a smoke test — add a person, start a shift, add a
product, 500 chairs in, refuse 900 out of 500, issue 40 then 10 to Ravi, return 45 of his
50 via a different person, and confirm the shortfall is attributed to Ravi and not to the
returner. Every one of those steps has been driven successfully against the real binary.

## Architecture, and the lines not to cross

```
main.go                startup, port hunt, browser launch
internal/register/     model, ids, fixture, arithmetic, allocation, Validate
internal/store/        atomic save and load
internal/web/          handlers, templates, static assets
```

- **Standard library only.** Zero third-party dependencies, and `go list -deps` proves it.
  If you think you need one, you have misread the problem.
- **No cgo, ever.** It would break the Windows cross-compile, which is the whole delivery
  mechanism. This rules out `mattn/go-sqlite3` and every native GUI toolkit.
- `internal/register` and `internal/store` must not import `net/http`.
- `internal/web` must not do arithmetic. It calls `internal/register`.
- **Bind `127.0.0.1` only.** Never `0.0.0.0`, never `:port`. `0.0.0.0` raises the Windows
  Defender Firewall dialog, and a non-technical user facing a security prompt is a failed
  handover.
- `internal/register/arith.go` must never see deleted records; `internal/register/log.go`
  must. That is why the log builder lives in its own file, and spec 03 has a grep
  asserting the separation. Do not merge them.
- Times in fixtures use `time.FixedZone("IST", ...)`, never `time.LoadLocation` — the
  tzdata files are absent from a `CGO_ENABLED=0 GOOS=windows` binary.

## Things that will bite you

- **Data loss is the worst possible outcome.** A day of entries at a live event cannot be
  recreated. The save path is temp file → fsync → rename current to `.bak` → rename temp
  over the real path, and a crash at any point must leave one good file or the other.
  Treat any change to `internal/store` as high risk and read its tests first.
- **A duplicate product silently halves the on-hand count** and nobody notices until it is
  too late. That is why product names are picked and never free-typed. A duplicate
  *person* is fine and must never be blocked — the asymmetry is deliberate and explained
  in `00-index.spec.md`.
- **The taker is not the returner.** When Suresh hands back chairs Ravi took, the
  shortfall stays against Ravi and every sentence says Ravi. Easy to get subtly wrong.
- **Over-issue and over-return re-check inside the `store.Update` closure**, against the
  register at that instant, not the number rendered on the page. Two browser tabs must not
  both spend the last ten chairs.
- **Deleting an issue that a live return points at** is invisible to `Validate` — the
  over-allocation check walks live issues while the return still counts its orphaned
  allocation, inflating on-hand. Spec 03 implements three invariants by design; this guard
  belongs in spec 11's delete path. Confirm it exists and is tested.
- **Never reword a user-visible string.** They have been through a plain-language review
  and are asserted verbatim in tests. If one reads badly, change the spec and the test
  together, and say why.

## Known open, nothing blocking

- **Two copies of the program can run at once**, on 8765 and 8766, showing two views of
  one file. Harmless in tests, plausible at a gathering if somebody double-clicks the icon
  twice. Worth deciding whether the second copy should open the first instead of starting
  its own. Not yet specced.
- **Unverifiable from Linux**, and needing a real Windows check before the event: browser
  launch via `rundll32`, `os.Executable()` resolving the data file beside the `.exe`,
  first-run from a folder the user cannot write to, how Edge renders `<input type="date">`
  and `datetime-local`, and the four JavaScript files in a real browser. Every scripted
  path has a server-rendered equivalent that the tests cover, so the register works with
  scripting off.
- `specs/07-inward.spec.md` open item 4: a supplier typed as `Sharma tent house` still
  becomes a second supplier row. Products are guarded, suppliers are not. Unconfirmed.
- The date hint says `Type the date like this: 03-09-2026.` while the field parses
  `YYYY-MM-DD`. Rendered as specced; someone should decide which is wrong.

## Working with this user

Recorded properly in `CLAUDE.md`, but the two that matter most:

- **Propose, do not interrogate.** One round of genuinely blocking questions, then act on
  stated assumptions. Repeated clarifying rounds read to him as going backwards, and he
  has said so.
- **Review agent output yourself and surface only what needs his judgement.** He said
  explicitly he will not review anything if nothing is flagged. Settle what you can,
  record it as a decision, and bring him the two or three that genuinely need him.

Keep messages short.
