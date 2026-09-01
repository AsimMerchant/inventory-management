# Spec: Global Desk Controls and Log Product Picker

## Objective

Make the two navigation actions needed during a busy handover explicit on every
screen: return to the Stock dashboard and change the person on duty. Also make the
`Which product` filter on `Who did what` behave as the same select-only autocomplete
used by the entry forms: typing `ch` lists matching products, but typed text is never
accepted as a product identity until a result is selected.

This is the first of three specs released together as `v1.1.1`. Spec 13's
multi-recipient issue is already complete and is not rebuilt here.

## Context

- Stakeholder decisions recorded 1 September 2026: Dashboard means `/stock`; it must
  be directly reachable from every screen; the on-duty person must be changeable from
  every screen and a change affects future actions only; the activity-log product
  filter must suggest existing products as the clerk types and require a selection.
- `design/store-register.html` already shows the current on-duty name in the chrome.
  Specs 04 and 05 make that name a link to `/shift`, but the requested actions were
  not explicit enough to be discovered reliably.
- Spec 06 owns `/api/products` and the shared select-only picker. Its matching rule is
  retained: case-insensitive substring, with names starting with the query first,
  alphabetical within each group, capped at eight.
- Spec 12 owns `GET /log`, which remains read-only. Its template already renders the
  shared picker but does not load `picker.js`; this spec completes and verifies that
  integration rather than creating another product-search implementation.

Amendments to prior contracts are explicit: spec 04's shared chrome gains the two
links; spec 05's on-duty-name link is supplemented, not removed, and its shift-write
semantics stay unchanged; spec 12's log template loads the already-contracted spec 06
picker script. No other prior contract changes.

## Contract

### Inputs

#### Shared chrome on every HTML page

`internal/web/templates/layout.html` renders these two links inside the shared chrome,
on every response drawn through `Server.render`, including `/shift`, `/stock`, all four
other tab pages, the three entry flows, correction/product-management pages, product
confirmation, and the not-found page:

```html
<a href="/stock">Dashboard</a>
<a href="/shift">Change person</a>
```

The exact visible labels are `Dashboard` and `Change person`. They are ordinary links,
not POST forms, and require no JavaScript. The current `<Name> · on duty` text remains
visible when a shift is live but is no longer the only way to discover `/shift`.

The links are rendered even on `/shift`. If no current-day shift exists, the existing
shift guard sends `/stock` back to `/shift`; the link does not bypass or weaken the
guard. Do not add a sixth tab, an admin page, a password, a role, or an end-shift action.

#### Changing the current person

`GET /shift` and `POST /shift/start` keep their spec 05 fields, statuses and validation.
Selecting `STF-0002` still atomically sets only:

```go
reg.OnDutyStaffID = "STF-0002"
reg.ShiftStartedAt = &now
```

and redirects 303 to `/stock`. It never rewrites `RecordedBy`,
`PersonInchargeName`, `TakenBackBy`, `CreatedBy`, `Change.By` or `Deletion.By` on any
existing record. There is no shift-history record and no activity-log row for a change
of duty. Every action begun after the successful switch reads the new on-duty staff
through the existing owning flow; actions already saved are never restamped.

#### Select-only product autocomplete on `GET /log`

`internal/web/templates/log.html` loads `/static/picker.js` after the form. The existing
shared picker renders:

- label `Which product`;
- visible input `name="productName"`, used only to search and display a selection;
- hidden input `name="productId"`, the only product identity applied to the filter;
- `data-mode="all"` and no add-new row;
- the existing `<noscript><select name="productId">` containing every live product.

Typing `ch` calls `GET /api/products?mode=all&q=ch`. Against T0 it returns, in order,
`Chairs — 390 on hand` and `Charcoal sacks — 12 on hand`. Arrow keys/Enter and mouse
selection set the hidden ID exactly as spec 06. Typing, then changing the visible text,
clears that ID. The log never filters by `productName` and never creates a product.

`GET /log?productId=PRD-0001` filters by Chairs and renders `Chairs` in the visible
input. `GET /log?productId=PRD-9999&productName=Chairs` treats the product filter as
unset. Submitting typed `Chairs` with an empty `productId` likewise means `Any product`;
it must not guess by name. Existing day, kind and person filters remain in the URL and
continue to combine with the selected product.

### Outputs

- Every rendered screen visibly contains direct `Dashboard` and `Change person` links.
- A shift change affects only actions recorded after the successful switch.
- The log's product suggestions are populated as the user types; only an existing
  selected product ID narrows the log.
- All existing screens, routes, tab order, entry wording and one/multi-person issue
  behavior remain unchanged.

### Side effects

- Following either chrome link performs no write.
- `GET /log`, `/api/products`, autocomplete typing and product selection perform no
  write.
- Only the existing successful `POST /shift/start` writes, using one atomic
  `store.Update`; refusal paths write nothing.

## Files to create or modify

- `internal/web/templates/layout.html` — explicit global links.
- `internal/web/templates/log.html` — load the existing picker script.
- `internal/web/shell.go` only if a small shared view field is needed; no new route.
- `internal/web/server_test.go`, `shift_test.go`, `log_test.go` — contract tests.
- `specs/00-index.spec.md` — build order and `v1.1.1` release grouping.

Do not modify `internal/register`, `internal/store`, `/api/products`, or the picker
matching algorithm for this spec.

## Required tests

`TestEveryRenderedScreenHasGlobalDeskControls` — with Suresh Kumar on duty, loop over
`/shift`, `/stock`, `/out`, `/inwards`, `/suppliers`, `/log`, `/inward/new`,
`/issue/new`, `/return/new`, `/entry/INW-0001/edit`, `/product/new`'s confirmation
response and `/not-a-route`; assert each rendered body contains links whose exact text
and destinations are `Dashboard` → `/stock` and `Change person` → `/shift`. The
not-found response remains 404.

`TestGlobalDeskControlsWorkWithoutJavaScript` — inspect the HTML only: both controls
are `<a href>` elements outside `<script>`/`noscript`, and neither is a form or button
requiring script.

`TestDashboardStillNeedsACurrentShift` — with no shift and with yesterday's stale
shift, `GET /stock` remains 303 to `/shift`; `/shift` still renders the Dashboard link
and no register bytes change.

`TestChangingPersonAffectsFutureActionsOnly` — start with Suresh on duty and an
existing Suresh-recorded `INW-0007`; switch to Anita using `POST /shift/start`, then
issue 10 chairs to Ravi. Assert 303 to `/stock`, the old inward still has
`RecordedBy == "Suresh Kumar"`, the new issue has
`PersonInchargeName == "Anita Rao"`, and no pre-existing provenance/audit field differs.

`TestChangingPersonCreatesNoLogRow` — compare `register.LogEntries` before and after
switching Suresh to Anita: slices are identical and there is no shift log kind.

`TestLogLoadsProductPickerScript` — `GET /log` contains the shared picker with label
`Which product`, `data-mode="all"`, a visible `productName`, a hidden `productId`, and
`<script src="/static/picker.js"></script>`; it contains no add-new-product row.

`TestLogProductPickerSuggestsTypedPrefix` — call
`GET /api/products?mode=all&q=ch` over T0 and assert the exact two ordered JSON labels
`Chairs — 390 on hand`, `Charcoal sacks — 12 on hand`; the same endpoint remains the
only product suggestion source used by the log picker.

`TestLogProductFilterRequiresSelectedID` — `GET /log?day=all&productName=Chairs`
shows all product kinds, while adding `productId=PRD-0001` shows exactly the nine T3
Chairs entries from spec 12. `productId=PRD-9999&productName=Chairs` behaves as no
product filter and never resolves the typed name.

`TestLogProductFilterKeepsOtherFilters` — selecting PRD-0001 on
`/log?day=2026-09-03&kind=went_out&q=98861` yields only ISS-0003 and ISS-0008, and
the generated filter/reset links retain the other active values.

`TestLogProductPickerHasNoScriptFallback` — the log HTML contains a select-only
`<noscript>` product list with all five T0 products, the selected ID when present, and
no text field that can become a product identity.

## Acceptance criteria

1. Every required test passes with `-race -count=1`; all existing tests remain green.
2. One shared layout change provides both links on every server-rendered screen; no
   handler carries a private copy of either control.
3. The shift guard, attribution fields and no-shift-history decision are unchanged.
4. `/log` uses `picker.html`, `picker.js` and `/api/products`; no second autocomplete
   or product matcher exists.
5. A non-empty `productName` with an empty/unknown `productId` cannot filter, create or
   rename any product.
6. No production package gains a dependency; the Windows cgo-disabled build succeeds.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestEveryRendered|TestGlobalDesk|TestDashboardStill|TestChangingPerson|TestLogLoadsProduct|TestLogProduct' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'Dashboard|Change person' internal/web/templates/layout.html
rg -n 'Dashboard|Change person' internal/web/templates --glob '!layout.html' # must print nothing
rg -n 'picker.js' internal/web/templates/log.html
rg -n 'store.Update' internal/web/log.go # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. The walkthrough has no explicit `Dashboard` or `Change person` labels. These exact
   strings implement the recorded stakeholder request and require the project's
   `plain_language_reviewer`; any rewrite must amend this contract and its tests.
