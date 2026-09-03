# Spec: Financial UI and End-to-End Browser Acceptance

## Objective

Integrate specs 17–20 into one small protected interface and require real end-to-end
browser evidence—from account setup through orders, receipts, money, settlement, audit,
printing and restart—before the feature branch may merge.

## Context

- The approved `design/store-register.html` predates financial work. Exact new strings
  in specs 17–20 trace to the user's 2 September 2026 decisions, not to that walkthrough.
- Ordinary staff are busy and untrained. The five inventory tabs remain; the inward and
  inward-correction flows additionally use the schema-5 shared supplier/other-party
  picker required by the user's 3 September 2026 decision. Other inventory flows remain
  visually/behaviorally unchanged except the neutral `Authorized login`.
- User explicitly requires real browser testing with Playwright/headless Chromium, not
  only handler tests. Project instructions require the Browser skill's Playwright
  interface and independent release gate.
- This spec depends on and releases specs 17, 18, 19 and 20 together. Partial release is
  forbidden because schema/auth/encryption, money and physical stock projections are one
  integrity boundary.

## Contract

### Inputs

#### Navigation and page map

Keep the ordinary five-tab inventory shell. Public chrome contains only its existing
inventory identity/controls plus `Authorized login`. An authenticated financial session
adds a clearly separated protected navigation group:

```text
Financial ledger | Orders | Transactions | Stock returned or sold | Financial activity | Logout
```

`Authorized people` and `Reusable lists` additionally appear for an administrator.
`Logout` is a POST form with CSRF, not a GET link. Every financial page shows the
authenticated display name/mobile and role; it never substitutes the on-duty inventory
person. Every finance flow has `Financial ledger` and `Logout` directly reachable.

Canonical routes are exactly:

| Method | Route | Page/action |
|---|---|---|
| GET | `/finance` | dashboard |
| GET, POST | `/finance/setup`, `/finance/setup/confirm` | first admin |
| GET, POST | `/finance/login`, `/finance/logout` | session |
| GET, POST | `/finance/activate`, `/finance/recover`, `/finance/recovery-key/new` | account access |
| GET/POST | `/finance/accounts` and spec-17 account actions | admin accounts |
| GET/POST | `/finance/lists` and spec-18 list actions | admin reusable values |
| GET, POST | `/finance/orders/new` | order create |
| GET | `/finance/orders`, `/finance/orders/{id}` | order list/detail |
| GET, POST | `/finance/orders/{id}/edit` | audited correction |
| POST | `/finance/orders/{id}/cancel` | audited cancellation |
| GET, POST | `/finance/product/new` | deliberate shared product creation |
| GET, POST | `/finance/movements/new` | one/batch money create |
| GET, POST | `/finance/movements/{id}/edit` | audited correction |
| POST | `/finance/movements/{id}/void` | audited void |
| GET | `/finance/journal`, `/finance/journal/print` | filtered/print transactions |
| GET, POST | `/finance/supplier-returns/new` | physical rental return |
| GET, POST | `/finance/sales/new` | physical sale |
| GET, POST | `/finance/settlements/{kind}/{id}/edit` | settlement correction |
| POST | `/finance/settlements/{kind}/{id}/void` | settlement void |
| GET | `/finance/settlements`, `/finance/obligations` | physical history/obligations |
| GET | `/finance/audit` | immutable financial activity |
| GET | `/finance/api/values` | authenticated suggestion JSON |

`GET /api/parties?q=<text>` is the shared public party-suggestion route owned by spec
18, not a protected finance route. An ordinary on-duty inventory request and a
confirmed finance session may call it. Its JSON contains suggestion IDs/names/labels
only; an authenticated read includes schema-4 vault party names before the first
schema-5 write without exposing why any party exists.

Wrong method is 405. Unknown protected entity is shell 404 with
`That financial record was not found.` and no decrypted neighbor data. All protected
GETs use `Cache-Control: no-store`, `Pragma: no-cache`,
`Referrer-Policy: no-referrer`, `X-Content-Type-Options: nosniff`, and
`Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self';
base-uri 'none'; frame-ancestors 'none'; form-action 'self'`. No financial value enters
URL parameters except explicit journal filters and opaque internal IDs on authenticated
pages. Amount, purpose, remarks and password are POST bodies. Party names may appear in
the query of the deliberately public `/api/parties` suggestion route; no financial
record or relationship is identified by that query or response.

#### Forms and refusal behavior

All forms preserve every non-secret submitted value after a validation refusal so the
user types it once. Password/setup/recovery fields are always blank after any response.
Validation is performed server-side in the order each owning spec lists. Every primary
button states the action (`Save order`, `Save transaction`, `Save supplier return`,
`Save sale`); destructive actions show impact/reason and require a second press.

JavaScript only adds typeahead keyboard/mouse behavior, repeatable rows and
`window.print()`. Every operation has a server-rendered fallback:

- reusable value/product selects plus explicit new-value/product confirmation;
- `Add another amount` submits a draft action which re-renders another row without
  saving any row;
- journal GET filters and print route;
- account setup, activation, corrections, cancellations and void confirmations.

Every destructive action in specs 17–20 uses its contracted consequence page and a
second CSRF-protected `confirm=yes` POST. JavaScript may reveal that page inline, but
may not skip it or make the first press mutate.

With JavaScript disabled, no internal ID is typed from memory: IDs appear only as values
of visible labelled `<select>` choices or hidden values created by server-rendered
links/forms. Finance product choice remains select-only; reusable finance values alone
may be deliberately typed new.

#### Minimal financial wording

The dashboard explains the separation once:
`Orders, money received or paid, and stock returned or sold are recorded separately.`

Use `Money paid`, `Money received`, `Net paid`; never `profit`, `loss`, `cash balance`,
`accounts payable`, `debit` or `credit`. A negative net renders with a minus sign and
means more money has been received than paid; it is not automatically labelled profit.
Never infer tax, invoice balance, settlement completion or ownership from entries.

Ordinary inventory pages contain no new supplier return/sale/payment/order rows. Their
Stock counts reflect the neutral public disposal projection, and their party picker may
show the contracted public name list. Protected finance pages may link back to
`Dashboard`; public pages never link directly to a protected subpage other than login.

### Outputs

- One binary supports inventory work with the shared party picker plus an authenticated
  financial area.
- Every agreed order/money/end-event path is reachable, auditable, encrypted and usable
  with or without JavaScript.
- A real browser run proves protection, stock arithmetic, date/time journal filtering,
  print output and restart persistence before release.

### Side effects

- Page navigation, suggestions, filters, print and failed/refused forms are read-only.
- Successful domain actions have exactly the atomic effects in specs 17–20.
- Browser acceptance creates all data only in a fresh temporary executable directory;
  it never uses or overwrites a real `store-register.json`.

## Files to create or modify

- `internal/web/templates/layout.html`, finance templates and static assets — protected
  navigation/forms/no-script/print behavior.
- `internal/web/server.go`, finance handlers and tests — complete route/security matrix.
- Browser acceptance is executed through the Browser skill; it adds no committed
  Playwright script, Node dependency or Go module.
- `HANDOFF.md` and `.agent-handoff/latest.md` — current spec/implementation/test status
  after each completed phase so another agent can resume without guessing.

## Required tests

`TestFinancialRouteTableAndSecurityHeaders` — every canonical path/method, 404/405,
no-store and exact security headers; no ordinary response gains protected navigation.

`TestEveryFinancialScreenHasProtectedNavigationAndLogout` — authenticated user/admin
matrix, exact labels/routes, role-only controls and POST/CSRF logout on every page.

`TestFinancialFormsWorkServerRenderedWithoutJavaScript` — first setup, invite/activate,
order with two lines/new product, batch split payment, refund, supplier return, sale,
correction/two-step void, journal/date/time filter and print through form/select/link semantics
only; never requires typing/reading an internal ID.

`TestFinancialRefusalsPreserveNonSecretInputOnly` — each form's first validation error
preserves party/product/quantity/amount/date/purpose/mode/reference/remarks and clears
password/setup/recovery secrets; no partial write or orphan reusable value.

`TestOrdinaryInventoryExperienceRemainsUnchanged` — byte-level/exact-string comparison
of every existing page before/after an unopened populated finance vault, except for the
approved `Authorized login` link, shared party picker/list and stock counts changed by
public disposals. Public party output contains no finance relationship or metadata.

`TestSharedPartyRouteHonorsBothAccessPathsAndNoLeakBoundary` — before any schema-5
finance write, authenticated Asha can query an old encrypted Sharma Events through
`/api/parties` without an inventory shift; after migration, on-duty Suresh sees the same
name/ID on `/inward/new` without finance login; an off-duty unauthenticated request still
obeys the ordinary shift guard; responses contain no amount/purpose/mode/account/mobile/
timestamp/audit/link fields.

`TestFinanceHeadersAndFormsContainNoExternalResource` — no CDN/network target, inline
secret, GET mutation, URL amount/party/remarks, autocomplete password, or cacheable
protected response.

`TestPlainLanguageContractsAndConfirmationSteps` — exact auth section/setup-code labels,
24-hour instruction, account/list/order/movement/settlement warnings and second-step
buttons; first step is byte-identical and second has CSRF+`confirm=yes`; obligations
heading/columns, immutable settlement-product guidance, direction refusal, settlement
selector, print actors and filter precedence all match specs 17–20.

## Acceptance criteria

1. Every required test from specs 17–21 and every pre-existing test passes under race.
2. A real native binary completes the browser scenario below in normal JavaScript mode,
   again for the marked no-script path, then after process restart.
3. Browser console has no uncaught error; every request stays on discovered
   `http://127.0.0.1:<8765..8785>`; no external request is made.
4. Raw main/backup inspection finds only the contracted public party IDs/names/aliases;
   all financial plaintext is absent and public disposal has no settlement kind,
   party-link or money.
5. Independent `plain_language_reviewer` has no blocking finding and independent
   `release_gate` reports `READY`; implementer/orchestrator reports cannot replace them.

### Browser acceptance scenario

Build a native Linux binary into a fresh `mktemp -d`, start it, discover the actual
loopback URL from stdout, and always stop that PID. Use only the Browser skill's
Playwright interface to drive visible/interactive state and inspect network/console.

1. On public Stock/shift screens, verify `Authorized login` exists and no finance menu,
   money text or protected API response is available. Public party suggestions may show
   names/IDs only. Direct `/finance` redirects to login; direct protected mutation
   without session/CSRF is 403/no write.
2. Set up first admin Asha Mehta / `98861 40023` with a test-only strong password. Save
   the displayed recovery key in test memory, confirm it, verify admin/financial menus.
3. Asha authorizes Rohan Das / `99001 34562` as Financial user; capture the one-time code,
   log out, activate Rohan with his independently chosen password, verify Rohan sees the
   full empty ledger but not account/list management.
4. Log in as Asha. Create shared party Sharma Events and an order containing 100 Tents on
   rent and 50 Chairs for purchase, estimated ₹25,000.00. Create Tents through the
   deliberate finance product confirmation. Verify protected order detail and zero-stock
   Tents on ordinary Stock. Confirm `/api/parties?q=sha` exposes Sharma's name/ID but no
   amount, order link, purpose, mode, account, audit actor or timestamp.
5. Log out, add/start inventory person Suresh Kumar, select Sharma Events from the shared
   picker, and record actual Tents receipts of 70 then 30 plus 50 purchased Chairs.
   Verify Stock 100 Tents/50 Chairs, both Tents inwards point at the same party while
   retaining their name snapshots, and the inward flow never asks for or reveals the
   order/expected/payment.
6. Log in as Rohan. Record ₹5,000.00 Money paid / Deposit / Online, then batch-save
   ₹5,000.00 Rent, ₹2,000.00 Freight and ₹2,000.00 Unloading labur (intentional typo) to appropriate
   supplier/payee parties, linked to the order/products. Use `Online payment` on one row,
   and create then correct a small row's party from `Sharm Events` to `Sharma Events` so
   the typo becomes unused. Verify Online and every new
   purpose/party immediately appears as another user's typeahead suggestion.
   Leave `Related stock return or sale` blank on these unrelated rows and verify its
   labelled empty/choice control is present.
7. Record three backdated protected movements on 1 January 2016 at 23:00, 23:20 and
   23:39, plus controls at 22:59 and 23:40. Filter the one day and exact
   `23:00`–`23:39` range; verify inclusive results. Open `/finance/journal/print`, verify
   only those three rows in ascending time, party/order/products/direction/purpose/mode/
   reference/amount/actor, filter label and print stylesheet; navigation/actions absent.
8. Log in as Asha, rename typo `Unloading labur` to `Unloading labour`, merge distinct
   mode `Online payment` into `Online`, delete now-unused typo `Sharm Events`, and verify Rohan's existing rows show
   corrected shared values while Financial activity preserves old/new/admin/time.
9. Return 40 Tents physically to Sharma. Verify public Stock becomes 60 and supplier
   obligation becomes 60; no automatic money row appears. Separately record ₹5,000.00
   Money received / Deposit refund by selecting that return under `Related stock return
   or sale`; Stock remains 60.
10. Sell 20 purchased Chairs to Patel Decorators. Verify public Stock becomes 30 and no
    automatic proceeds exist. Record ₹4,000.00 then ₹6,000.00 Money received linked to
    the sale through the required labelled selector; stock remains 30 and journal has
    both installments.
11. Correct one movement amount/purpose and supplier-return remark; void a deliberately
    mistaken sale of 5 Chairs. Verify current totals/stock, original values, void reason,
    immutable Asha/Rohan mobile identities and chronological financial audit.
12. Cancel an additional paid-but-undelivered 100-chair order, record the equal incoming
    refund, and verify both rows plus zero net for that order; nothing is deleted and no
    stock changes.
13. Explicit Logout removes the financial menu/data immediately and protected back-button
    responses are unavailable. The exact 14:59-active/15:00-expired boundary remains a
    deterministic clock-controlled integration test required by spec 17; this browser
    run must not claim a real-time idle test without actually waiting 15 minutes.
14. Stop the exact server PID. Inspect JSON: schema 5, readable inventory/disposal and
    public party rows containing only `id`, `name`, optional `previousNames` and optional
    `mergedIntoId`; no distinctive plaintext amount, purpose, mode, financial link,
    account/mobile or audit provenance. Restart the same binary/directory; verify all
    stock totals and party suggestions without finance login, then authenticate and
    repeat key order/journal/audit/settlement assertions.
15. In a new browser context with JavaScript disabled, log in and complete one existing-
    value payment, one new-purpose payment through `Or add a new one`, exact-time filter,
    print view and logout entirely through server-rendered controls.

Record pass/fail, exact binary commit, commands, discovered URL, browser engine/version,
assertion count, console/network findings and any diagnostic screenshot paths in
`HANDOFF.md`. Screenshots must not include the recovery key or password.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestFinancialRoute|TestEveryFinancial|TestFinancialForms|TestFinancialRefusals|TestOrdinaryInventory|TestSharedParty|TestFinanceHeaders|TestPlainLanguageContracts' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
file /tmp/register.exe
go list -deps ./... | grep -v '^storeregister' | grep -v '^vendor/' | grep -v '^crypto/internal' | grep '\.' # must print nothing
rg -n '0\.0\.0\.0|net.Listen\("tcp", ":' main.go internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n 'https?://' internal/web/templates internal/web/static # must print nothing
rg -n 'profit|loss|cash balance|accounts payable|\bdebit\b|\bcredit\b' internal/web --glob '*.go' --glob '*.html' --glob '!**/*_test.go' # must print nothing
```

## Open

1. Real Windows behavior—browser launch, executable-relative data path, Edge printing
   and folder permissions—still requires a final Windows 11 check. Linux native browser
   acceptance plus the cgo-disabled Windows build cannot prove those operating-system
   integrations.
2. No PDF/CSV export, invoice image/file attachment, tax/GST calculation, multi-currency,
   SMS, network sync or profit calculation was requested; none is included.
