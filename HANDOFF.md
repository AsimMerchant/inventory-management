# Handoff

## Active work — financial ledger (started 2 September 2026)

Work is in progress on branch `feature/financial-ledger`, based on released `v1.1.1`
commit `f619ab7`. No Go implementation existed at the initial checkpoint. The clean
baseline was independently reproduced on this branch with `go test ./... -race
-count=1`, `go vet ./...`, and the cgo-disabled Windows amd64 build; all passed.

The user has explicitly superseded the old "no authentication, money or supplier
return" product decisions for a protected financial area only. Ordinary inventory
work remains unauthenticated and must reveal no financial content. A neutral
`Authorized login` leads to individual mobile-number/password accounts; the first
account is administrator, administrators authorize later accounts, and financial
menus appear only after server-enforced login. Financial records require encryption,
audited corrections/voids, recovery, and atomic persistence.

Agreed ledger scope includes multi-product rent/purchase orders; optional estimated
or exact agreed totals; independently timed outgoing payments and incoming refunds or
sale proceeds; reusable typeahead values for supplier/payee, purpose and payment mode;
admin rename/merge and safe typo deletion; partial physical receipt through the
unchanged inward flow; deposit refunds; physical rented-goods returns to suppliers;
sales of purchased stock; and a protected printable transaction journal filtered by
day, inclusive date range, or exact local date/time range. Money movements and
physical settlement movements are separate because they may occur at different
times. The reviewed contracts are specs 17–21: protected vault/accounts, orders and
reusable values, money/audit/printable journal, supplier returns and sales, and the
integrated browser acceptance gate.

### Deferred acquisition-basis follow-up — not in specs 17–21

The user later clarified that acquisition basis should not become one fixed `Borrowed`
enum. A future, separately specified feature should keep built-in suggestions for Rent
and Bought while allowing a user to type a new acquisition-basis value once and select
it from suggestions everywhere thereafter. Reusable basis values will need the same
administrator typo maintenance as other shared financial values: rename, merge, and
safe deletion when unused. No current implementation or spec contract changes for this
follow-up.

The user then settled the important semantic boundary: acquisition basis and final
disposition are separate. Inventory staff only receive stock and ensure issued goods
return to the inventory pool. After the event, an authorized user explicitly records
what finally happened—returned, sold, transferred, discarded, donated, or a future
custom outcome—and records related money separately. No return/sale/keep behavior may
ever be inferred from a custom acquisition-basis name such as `Borrowed`, `Lent free`,
or `Sponsored`.

### Build progress against specs 17–21

`.agent-handoff/` is gitignored, so it does not survive a fresh clone. **This section is
the resume artifact.** Keep it current after every committed slice.

| Spec | Slice | Commit | State |
|---|---|---|---|
| 17 | Vault, accounts, sessions | `cdc431c` | **Done.** All 16 required tests pass under `-race`. |
| 18 | 1. Order/value model, IDs, validation, rupee parser | `0b9cbb9` | **Done.** register-level tests pass. |
| 18 | 2. Reusable values: typeahead, admin `/finance/lists` | `f162427` | **Done.** 3 required tests pass. |
| 18 | 3. Orders: create, list, detail, `POST /finance/product/new` | `c484970` | **Done.** 6 required tests pass. |
| 18 | 4. Order edit and cancel, the six-row `FinanceChange` table, suggestion ordering, docs | `b97023b` | **Done.** All 12 required tests pass. |
| 19 | 1. Movement model, totals, journal filters, checked int64 money | `f7a831e` | **Done.** register-level tests pass. |
| 19 | 2. Recording money, single and batch | `f70d75d` | **Done.** 7 required tests pass. |
| 19 | 3. Corrections and voids | `d6551ea` | **Done.** |
| 19 | 4. Dashboard, journal, print view, financial activity | `d6551ea` | **Done.** All 13 required tests pass. |
| 20 | 0. `Register.Disposals` and the new `OnHand` — **alone, as a regression canary** | `7e01240` | **Done.** Whole suite green with an empty slice. |
| 20 | 1. Settlement types and pairing validation | `206e705` | **Done.** |
| 20 | 2. Allocation, party aliases and supplier obligations | `206e705` | **Done.** 6 required register tests pass. |
| 20 | 3. Supplier returns, sales, settlement and obligation screens | `47a1758` | **Done.** |
| 20 | 4. Settlement edit/void, inward guards, product cascade | `47a1758` | **Done.** All 13 spec-20 web tests plus 6 register tests pass. |
| 21 | 1. Protected navigation, route and header matrix | `9dcda6e` | **Done.** All 6 required tests pass. |
| 21 | 2. Plain-language review, two-step confirmations, paired spec/test amendments | `643b159` | **Done.** 13 new contract tests; all 74 required tests across 17–21 exist and pass. |
| 21 | 3. Browser acceptance run, release gate, docs | in progress | **Scenario passed** — see the run record below. `release_gate` outstanding. |

**Spec 21 makes two things blocking that spec 18 did not.** Its acceptance criterion 5
requires an independent `plain_language_reviewer` with no blocking finding *and* an
independent `release_gate` reporting `READY`; neither can be replaced by the
implementer's own report. Criterion 2 requires the browser scenario actually executed
against a real native binary, in normal mode, again with JavaScript off, and again after
a restart.

**Decision taken before spec 20 starts:** spec 20's central invariant pairs a protected
settlement with a public disposal, which `ValidateFinance(f)` cannot see. Rather than
widen that signature across every spec-17 call site mid-feature, the pairing check lives
in its own `ValidatePairing(reg, f)` called beside the two existing validators inside
`UpdateFinance`.

Spec 17 arrived from the previous session mid-refactor and did not compile: the request
was being threaded into `Server.page` so the chrome can tell whether a finance session is
live, and four `render*` helpers still referred to an `r` they did not take. Completing
that threading was the whole fix; no spec-17 behaviour was changed.

**The financial screens are placeholder HTML.** They work and are asserted by tests, but
they are unstyled and have not been through `plain_language_reviewer`. Spec 21 makes them
real. Do not judge the feature by opening it before then.

#### Browser acceptance run record (spec 21 criterion 2)

Binary built from `fede98f` plus the fixes below, run from a fresh `mktemp -d`,
Chromium via Playwright 1.62.1, discovered URL `http://127.0.0.1:8765`.

| Pass | Result |
|---|---|
| Steps 1–13, normal JavaScript | **85 checks, 0 failures, 0 console errors, 0 external requests** |
| Step 14, file on disk after stopping the server | schema 3; products, inwards and disposals readable; `finance` holds only `vaultVersion`, `keySlots`, `recovery`, `nonce`, `ciphertext` |
| Step 14, restart with nobody logged in | Tents **60**, Chairs **30** — correct after a supplier return, a sale and a void — and no finance menu anywhere |
| Step 15, JavaScript disabled | **17 checks, 0 failures**: login, an existing-value payment through the selects, a brand-new purpose through `Or add a new one`, exact-time filter, print view and logout |
| Step 12, cancel a paid undelivered order | **12 checks, 0 failures**: the two-step cancellation warns that payments will not change, the original ₹10,000.00 stays on the journal, the equal refund is its own incoming row, and Chairs stock is unchanged throughout |

**Two names are in the public file, and both are deliberate.** Do not "fix" them:
`inwards[].supplier` is ordinary inventory data the desk types, and
`products[].createdBy` carries the authenticated display name because spec 18's contract
requires it when a product is created through the finance route. No money, mobile,
party list, settlement kind or record ID appears in plaintext. Everything else on the
protected-value list was checked and is absent.

**Every step of the scenario has now been driven in a real browser.** The idle-expiry
boundary is deliberately left to the deterministic clock test, as spec 21 step 13
itself directs: the browser run must not claim a real-time idle test without waiting
fifteen minutes.

One more selector fact: the order detail's `Review cancellation` and the settlement
list's void control both sit inside a `<details class="danger">` that starts closed.

#### Browser acceptance: how to run it

The script is scratchpad-only (spec 21 forbids committing one). Rebuild, copy into a
fresh `mktemp -d`, run it there, and drive `http://127.0.0.1:8765`. Playwright 1.62.1
lives in the npx cache; set
`NODE_PATH=/home/asim/.npm/_npx/e41f203b7505f1fb/node_modules`.

Selector facts learned the hard way, all of which cost a run each:

- **The chrome's Logout is the first submit button on every protected page.** Any bare
  `click('button[type=submit]')` logs you out. Name the form, or click by button text.
- The order form's **Save order** is *not* the first submit in its form either — the
  per-line "add this as a new product" button comes first and carries `formaction`.
  Click `button:has-text("Save order")`.
- Setup, login and activate forms **post to the current URL and carry no `action`**, so
  select them by a field only they have.
- Basis radios are **indexed per row**: `basis-0`, `basis-1`.
- `<details class="add">` on `/shift` is **already open** on an empty register; clicking
  the summary closes it. Check `.open` first.
- Adding a person is not putting them on duty: **select the radio, then press Start
  shift**, or every ordinary page keeps redirecting to `/shift`.
- With script on there is **no `<select>` in the DOM at all** for pickers — the
  `<noscript>` fallback is not parsed. Drive the real picker: type, wait, click the row.
- A fresh binary has **an empty catalogue**. Spec 21 step 4 assumes Chairs exists; it
  does not, so create both products through the deliberate finance route.

#### Traps this build has already hit

- **A screen can be impossible to use while every handler test passes.** The supplier
  return form renders its product list from the server, and that list depends on which
  supplier the goods came from. Choosing a supplier in the browser cannot refill it, so
  the screen could never be completed. The handler tests missed it because they post the
  supplier and product together in one request, which no person can do. There is now a
  plain submit, `Show what can go back to this supplier`, that redraws the form with the
  list and writes nothing; `TestSupplierReturnListsProductsAfterChoosingSupplier` pins
  it, including that merely asking creates no party.
- **The whole handler suite can miss a defect that only a running binary shows.**
  Every web fixture starts from `WalkthroughT0()`, which has a live shift, so nothing
  caught that the finance order form's product picker asked `/api/products` — a route
  behind the *inventory* shift guard. With nobody on duty at the desk, a financial user
  saw an empty picker and could not choose a product at all. Fixed by giving the picker
  a `data-endpoint` and adding the session-gated `/finance/api/products`;
  `TestFinanceProductPickerWorksWithNoInventoryShift` pins it with the shift cleared.
  **Run the real binary as well as the suite.**

- `deepCopyFinance` was a shallow copy. `FinanceOrder` carries `Lines`, `Changes` and a
  `*int64`; sharing those backing arrays would let a **refused** transaction leave
  half-applied edits in the live decrypted data. It now deep-copies all three.
- Spec 18's float grep has no `!**/*_test.go` exclusion, unlike its `Products = append`
  grep on the line above. A float literal in a *test* therefore fails the gate. The tree
  is clean of floats today, tests included; keep it that way rather than amending the
  spec.
- Resolution of a typed party/purpose/mode must happen **inside** the `UpdateFinance`
  closure, not before it, or two simultaneous posts create two rows saying the same
  thing.
- `FinanceOrderLine.ID` is unique across the whole vault, not within one order, so
  `NextID("OLN")` scans every line of every order.
- **Resolving a shared value in its own transaction saves the file.** The settlement
  screens first resolved the party in a separate `UpdateFinance`, then recorded the
  settlement. A settlement refused for exceeding its cap had therefore already rewritten
  the register and could leave a newly created party behind with nothing pointing at it.
  Resolve inside the same write that stores the record.
- **A supplier can have goods here before finance has ever heard of them.** Inventory
  staff type the supplier straight onto the inward, so the return screen must work out
  availability from the typed name (`SupplierReturnAvailableByName`) as well as from a
  selected party, or the first return to any supplier reads "Only 0 can be returned".
- **Adding a field to `Register` breaks three things at once**, and the canary commit is
  how you find out cheaply: the golden `internal/store/testdata/walkthrough-t0.json` has
  to be regenerated, `register.WalkthroughT0()` must set the new slice to empty or a
  register in memory stops comparing equal to the same register read back off disk, and
  `TestEmptyRegisterEncodesEmptyArrays` catches a nil slice encoding as `null` in a file
  that is meant to be human-readable. Regenerate the golden file by running a throwaway
  `main` inside the module (an external one cannot import `internal/`).
- **`Store.Read` and `Store.ReadFinance` both take the same non-reentrant mutex.**
  Nesting them deadlocks the request with no error and no test failure — the run just
  hangs. Any screen needing the inventory record and the vault together must use
  `Store.ReadBoth`, which hands over both under one lock. This bit the order list and
  detail screens.

#### What spec 20 inherits

- `register.FinanceLineIsReferenced` still returns `false`. Spec 19 chose not to fill it
  in because a movement points at an order *line* only through `OrderLineIDs`, and spec
  18's guard is about a line disappearing under a ledger entry. **Spec 20 or a follow-up
  must make it report `len(m.OrderLineIDs) > 0 && contains(m.OrderLineIDs, lineID)` over
  live movements**, and the two partial spec-18 tests then become complete.
- `register.FinanceValueIsUsed` now covers order party and movement party, purpose and
  mode. Every settlement field naming a value must be added to it in spec 20.
- `Store.ReadBoth` is the only safe way to read the inventory record and the vault
  together.
- The placeholder handlers in `internal/web/finance_settlements.go` exist so the
  dashboard has no dead links. Spec 20 replaces all three.

#### What spec 19 inherited

- `register.FinanceLineIsReferenced` returns `false` and must start reporting whether a
  movement points at an order line. `lineRefusal` in `internal/web/finance_orders.go`
  already calls it and already carries the exact wording.
- `register.FinanceValueIsUsed` currently checks only `FinanceOrder.PartyID`. Every new
  movement field naming a party, purpose or payment mode must be added to it, or an
  administrator will be able to delete a value a movement still points at.
- `Store.ReadBoth` is the way to read the inventory record and the vault together.
- `financeAuditFor` in `internal/web/finance_orders.go` and `store.FinanceAudit` do the
  same job at two layers; spec 19 should use whichever layer it writes in rather than
  adding a third.

#### Known gap: two spec-18 tests depend on spec 19

`TestCancelPaidUndeliveredOrderKeepsHistory` and
`TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear` both require a money movement
pointing at an order line, and spec 18 explicitly forbids declaring the movement type.
The refusal path and its exact wording `This product is already used by a ledger entry.`
ship in spec 18 behind `register.FinanceLineIsReferenced`, which returns `false` until
spec 19 fills it in. Both tests are written and cover everything reachable today; their
movement-linked assertions are the named gap. **Spec 19 must implement that predicate and
complete both tests.**

Continuation details are also kept in `.agent-handoff/latest.md` after each milestone,
but that file is gitignored: never make it the only record of anything.

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
4. The numbered specs, `01` through `13`.

The specs are precise and have been reviewed twice: once for correctness, once by a
plain-language pass over every user-visible string. **Follow them exactly.** Where a spec
and your instinct disagree, the spec wins. Where a spec is genuinely wrong, implement the
Contract section and *report the defect* — every previous pass found real ones that way,
and that reporting has been more valuable than the code.

## State

| Specs | What | Status |
|---|---|---|
| 01–09 | Core model, persistence, arithmetic and entry flows | **Released in `v1.0.0`** |
| 10–12 | Read-only views, corrections and activity log | **Released in `v1.0.0`** (`c0adcdc`) |
| 13 | One issue may name multiple joint recipients | **Released in `v1.0.1`** (`77a40f0`) |
| 14–16 | Global desk controls, product rename and cascading delete, issue challans and return search | **Released in `v1.1.1`** (`3bc9535`) |

### Before the new `.exe` goes on the laptop — do this in order

`v1.1.1` writes schema 2. `v1.0.1` cannot read it, and **does not refuse it cleanly**:
its `store.Open` treats a version mismatch exactly like a damaged file, copies the main
file aside and falls back to `.bak`, which directly after the first `v1.1.1` save is the
schema-1 pre-upgrade file. The old program therefore opens on pre-upgrade data behind
the ordinary "Today's register was damaged" banner, and its next save writes schema 1
back over the main file. Nothing is destroyed — the schema-2 file is kept as
`store-register.json.corrupt-<timestamp>` — but the banner blames damage when the real
cause is that somebody ran the old program, and nothing on screen says so.

This cannot be fixed in `v1.1.1`; the defect is in the already-released `v1.0.1` reader.
The user's decision, 1 September 2026, is to handle it procedurally:

1. Copy `store-register.json` somewhere safe.
2. Delete the old `.exe` from the laptop **and** from the pen drive.
3. Put the new `.exe` in its place.

Verified against real binaries by the `v1.1.1` release gate. With no `.bak` beside it,
`v1.0.1` refuses correctly and touches nothing; after two `v1.1.1` saves both files are
schema 2 and it refuses correctly again. The exposure is the window in between.

### `v1.1.1` verification, all reproduced independently by the release gate

372 tests pass under `-race -count=1` with no failures and no skips; `go vet` is clean;
the cgo-disabled Windows amd64 build produces a PE32+ executable; all 57 tests named as
required by specs 14, 15 and 16 exist and pass; every must-print-nothing grep is silent.
`internal/register` statement coverage is 95.4% against the documented 95% minimum —
down from 97.1%, and the uncovered statements are guard returns the web layer checks
before calling, plus the `Validate` abort at `product.go:145`.

Spec 16's browser acceptance scenario passed in full: all 9 steps through Playwright
against a real native binary, 67 checks plus 11 more after restarting the binary against
the saved file, and again server-rendered with no JavaScript at all. Under real
concurrency at a live binary, 40 simultaneous issues of 10 against 100 on hand accepted
exactly 10 and refused 30; 20 simultaneous returns against a 10-chair holding accepted
exactly 1; a delete racing 15 issues on the same product left no live record pointing at
a deleted product.

`v1.0.1` is the selected version for spec 13. The strict release matrix passes on the
current tree: the full race suite, vet, Windows amd64 cross-compile, store and feature
tests, architecture/vocabulary greps, and 97.1% `internal/register` statement coverage
(the documented minimum is 95%).

The read-only `plain_language_reviewer` checked 175 built strings and found only the
known no-supplier wording conflict recorded below; no code change is warranted while
the two specs disagree. Native Linux binaries passed both the original end-to-end smoke
and a spec 13 smoke that stored one 30-chair issue for Ravi, Amit and Suresh, counted it
once, found it through Amit, and preserved the ordered recipients in JSON. The spec 13
contributor reports that the enhanced workflow passed on Windows; Codex independently
verified the cross-build and automated matrix but did not reproduce that manual run.

Known spec-text defects found while implementing the required tests:

- Spec 15 stated that a `v1.0.1` executable refuses a schema-2 file. It does not; see
  the upgrade steps above. Corrected in `83f0a0b`.
- Spec 15's verification commands grepped `internal/web/products.go` for `store.Update`,
  which prints nothing because the code calls `s.st.Update`. The guard was present the
  whole time; the grep could not see it. Corrected in `83f0a0b`.
- Spec 00's greps used unquoted `--include=*.go` globs, which this project's fish shell
  expands before `grep` sees them, so they failed outright rather than passing.
  Corrected in `83f0a0b`.

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

- **Nine wording findings against `v1.1.1` strings are unactioned.** The
  `plain_language_reviewer` flagged `Dashboard` (a second, differently-named link to
  `/stock`, which the tab bar already labels `Stock`), `Change person` (reads as "change
  who is taking this" mid-entry), the cascading-delete impact block (`Received entries:`
  / `Issue entries:` / `Return entries:`, and "the working register", a phrase on no
  other screen), `Outstanding entries with this challan` and `No outstanding issue
  matches that challan number.` ("outstanding" is a finance word that reads as
  "excellent" to a second-language reader, while the row below says plain `20 chairs
  out`), and `Pick this holding` (does not say what tapping it does). The release gate
  added a tenth: `Currently out: 1 chairs` on the delete screen violates normative
  decision 10. All are wording, none are defects; each needs its spec and its verbatim
  test amended together. Put to the user 1 September 2026, deferred, not declined.
- **The save-time recheck tests are not load-bearing.** The release gate disabled, one
  at a time, the in-closure `impactVersion` recheck, the in-closure rename duplicate
  check, `Validate` inside `DeleteProductCascade`, the in-closure `CheckIssue`, and
  `Product.Changes` deep-copying — and the full suite stayed green for all five. The
  code is correct and was proved so empirically under real concurrency, but every
  "rechecks at save time" test stages its conflict *before* the POST, so nothing
  distinguishes a check inside the `store.Update` closure from one outside it. A future
  refactor could move one out and no test would notice. This is the code that stops two
  browser tabs spending the last ten chairs; worth strengthening.
- **Quantities have no upper bound**, in the field since `v1.0.0` and untouched by
  `v1.1.1`. The inward field is `min="1"` with no `max` and there is no server-side
  limit; the gate overflowed int64 and drove on-hand to a large negative number. The
  realistic failure is not the overflow but one fat-fingered `50000000` inward silently
  corrupting the headline number. Needs its own spec.
- **The activity log's person filter records the returner, not the taker**
  (`internal/register/log.go:114`). Searching the log for the taker misses the return
  that put their goods back. The release gate confirmed this is what
  `specs/12-activity-log.spec.md:115` actually prescribes, and that `/out` attributes
  the shortfall correctly, so no number is wrong — it is discoverability only. Changing
  it means amending spec 12.

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
