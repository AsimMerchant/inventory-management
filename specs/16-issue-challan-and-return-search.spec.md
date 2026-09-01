# Spec: Issue Challans and Return Search

## Objective

Record an optional challan number on each issue and use any part of that number to find
outstanding stock when it comes back. A challan number is a reference copied from the
paper challan book, not a unique ID and not a stored group: three separately entered
issues for cables, chairs and tables may all say `452`, and the same value may also be
used for another recipient group.

This is the third of three specs released together as `v1.1.1`; it depends on specs 14
and 15. Spec 13's multi-recipient quantity semantics remain unchanged.

## Context

- Stakeholder decision recorded 1 September 2026: challan is optional; one number may
  cover multiple products and multiple recipient groups; the person in charge is
  responsible for typing the chosen number independently on each issue; different
  numbers for the issues are also valid; no automatic reuse is wanted.
- Recorded follow-up decisions: challan corrections are audited; the activity log can
  be searched by challan; return search is case-insensitive partial matching and shows
  every outstanding match with product, outstanding quantity, recipient/group, issue
  time and challan before the existing holding/product return guards apply.
- The approved walkthrough has an inward `Challan no.` but no outward-challan field.
  The paper challan book described by the stakeholder is the authority for this
  additive outward field.
- A challan never replaces `Issue.ID`, `ProductID`, return allocations or the
  select-only product/person rules. The clerk never sees or types an internal ID.

Amendments to prior contracts are explicit: spec 01's `Issue` gains one optional
field; specs 08 and 13's one issue POST stores it without changing quantity or
recipients; spec 09 gains an alternative finder but keeps allocation rules; spec 10
shows the reference on live issue rows; spec 11 makes it correctable through the shared
audit phrase; spec 12 derives/displays/filters it. No `Return` field or challan entity
is added.

## Contract

### Inputs

#### File format and register API

Keep every existing `Issue` field and add immediately after `Quantity` in
`internal/register/model.go`:

```go
ChallanNo string `json:"challanNo,omitempty"`
```

`SchemaVersion` remains 2 from spec 15. Schema-1 files migrate through spec 15's
read-compatible path and load `ChallanNo == ""`; old one-person and multi-person
records behave unchanged. An empty challan is omitted when saved.
Non-empty challans are stored as `register.CleanName(input)`: outer whitespace is
trimmed and internal whitespace collapses, while letters, digits, `/`, `-` and case are
preserved for display. There is no format validation, uniqueness check, sequence,
`Challan` type, challan slice, or relationship among records sharing text.

Add in `internal/register/challan.go`:

```go
type ChallanMatch struct {
    IssueID       string
    HoldingIssueID string
    ProductID     string
    ProductName   string
    Outstanding   int
    Recipients    []IssueRecipient
    RecipientLabel string
    IssuedAt      time.Time
    ChallanNo     string
}

func FindOutstandingByChallan(*Register, string) []ChallanMatch
```

For a non-empty cleaned query, inspect every live issue whose product is live,
`OutstandingOnIssue > 0`, and whose `FoldKey(ChallanNo)` contains `FoldKey(query)`.
Issues with an empty challan never match. Return one row per matching issue, even when
several share a challan, product, recipient set or holding. Copy recipients and derive
the full `RecipientLabel` through spec 13.

`HoldingIssueID` is the anchor returned by the existing `JointHoldingForIssue`: the
issue itself for a joint holding and the oldest outstanding issue for a solo person's
holding. This is an internal link value only. Sort matches by folded challan, then
`IssuedAt`, then issue ID, all ascending. The function performs no I/O and reads no
clock.

Selecting a match hands its `HoldingIssueID` and `ProductID` to the existing return
flow. The selected holding is re-derived, and selecting that product still includes
every outstanding line of that product in that holding, exactly as specs 09 and 13
require. Thus, if Ravi has two solo Chairs lines under different challans, finding one
challan gets the clerk to Ravi's Chairs holding; the return form then shows and defaults
to the complete outstanding Chairs quantity. Challan text never limits or rewrites a
return allocation.

#### `GET, POST /issue/new`

Directly after the quantity field and before recipient fields, render:

```text
Challan no. (optional)
Type it from the paper challan.
```

The text input is `name="challanNo"`, `autocomplete="off"`. It is blank on every new
GET, including after a previous successful issue and when product/recipient query
parameters are present. Do not copy it from the latest issue or another issue sharing
the product/people. A refusal re-renders and preserves only the value posted in that
request.

On successful POST, store `ChallanNo: register.CleanName(r.FormValue("challanNo"))`
on the one issue already appended by specs 08/13. It is optional and adds no validation
failure. The success redirect/status and stock banner are unchanged; the banner does
not repeat the challan.

The value is read and assigned inside the same `store.Update` callback that re-runs
`CheckIssue`. Recipient count still neither divides nor multiplies quantity.

#### Issue correction and ordinary displays

`GET /entry/{issueID}/edit` adds the same field, prefilled, after `How many` and before
recipient fields. It is editable. A changed value appends:

```go
Change{
    At: now, By: onDuty.Name, Field: "challan", Label: "Challan no.",
    From: oldChallan, To: newChallan,
}
```

`changePhrase` renders:

- both filled: `Changed the challan no. from 452 to CH-452`;
- empty to filled: `Added a challan no.: CH-452`;
- filled to empty: `Removed the challan no. that said: CH-452`.

Changing challan does not run stock arithmetic beyond the correction path's mandatory
`Validate`; it changes no quantity, recipient, allocation or timestamp. Re-resolve the
issue inside `store.Update`; a deleted issue/product is refused by existing guards.
When one edit changes quantity and challan, append the quantity change first, then the
challan change, then recipient fields in their existing form order.

Every live issue line on `/out` and every selected holding line on `/return/new` adds
`Challan no. <value>` in `sm` type when non-empty. No placeholder is shown when empty.
Product rename resolves the current product name; product cascade removes the row from
these working screens.

#### Return search by challan

`GET /return/new` keeps the existing person search and adds, directly below it:

```text
Find by challan no.
Type any part of the challan number.
```

The GET text field is `name="challan"`; no hidden/internal ID is visible. Both finders
are alternatives and neither is required before the user starts typing. A non-empty
`challan` displays `FindOutstandingByChallan(reg, challan)` under heading
`Outstanding entries with this challan`.

When both `q` (person search) and `challan` are non-empty in a hand-built/no-script
request, challan search takes precedence and no person holding is auto-selected. The
client enhancement clears the other visible finder when typing, but this server rule is
authoritative. Selecting a challan result drops `q`; clearing challan returns to the
unchanged person-search behavior.

Each match is one tappable/link row containing these exact shapes:

```text
Challan 452
20 chairs out
Ravi Menon and Amit Sharma · taken 3 September · 1:24 pm
Pick this holding
```

The link is server-rendered and works without JavaScript:

```text
/return/new?challan=452&holdingIssueId=<anchor>&productId=<product>
```

The desk sees no anchor/product ID. Preserve the typed challan in the visible field.
The match's `Outstanding` is that matching issue's own outstanding quantity. After the
tap, the existing selected-holding/product section makes the full quantity being
returned explicit and prefilled; the clerk is never expected to remember the number
from the search row.

No matches renders `No outstanding issue matches that challan number.` Settled,
deleted, empty-challan and deleted-product issues never appear. Matching is
case-insensitive partial: `45` finds `452`, `CH-452` and `452-A`; `ch-452` finds
`CH-452`. The same query may return different products and recipient groups.

GET and POST must never trust `holdingIssueId`, `productId`, `issueIds`, recipients,
quantity or challan from the match page. Re-derive the live holding and selected
product using spec 13, and inside the return's single `store.Update` callback re-run
`JointHoldingForIssue`, `CheckReturn` and allocation. A holding changed/settled/deleted
after search gets the existing exact refusal
`That holding has changed. Pick it again from the list.` and no write.

The challan is a finder only. `Return` gains no challan field and allocations continue
to store only issue IDs and quantities.

#### Activity-log display and search

Add to `register.LogEntry`:

```go
ChallanNo string
```

Populate it on `LogCameIn` from `Inward.ChallanNo` and on `LogWentOut` from
`Issue.ChallanNo`. Copy it to those records' correction/deletion rows. Other kinds have
empty challan except a challan correction row, which keeps its issue/inward challan
context as `changesOf` already keeps product/person context.

Add to `register.LogFilter`:

```go
ChallanQuery string
```

When non-empty, a row matches only if `FoldKey(e.ChallanNo)` contains
`FoldKey(ChallanQuery)`. For a `LogCorrected` row whose `Change.Field == "challan"`,
also match `Change.From` and `Change.To`; therefore a removed or replaced old challan
remains findable by the text it used to carry. It combines with day, kind, person and
product. This searches both incoming and outgoing challans. Deleted record rows remain
searchable.

`GET /log` adds a GET field after `Which product`:

```text
Challan no.
Type any part of the challan number.
```

Its name/query parameter is `challan`. Invalid/empty means no challan filter. Every
filter/reset link preserves a non-empty challan. A visible reset link reads
`Any challan` when active. The filtered empty advice becomes
`Tap Every day, Everything, Anybody, Any product and Any challan.`

Issue log rows with a non-empty value carry `Challan no. 452.` in `sm` type. Inward
rows continue to derive their main sentence unchanged and gain the same note. Empty
values render no challan line. A correction row shows the exact `changePhrase` above;
do not create a second correction wording implementation in `log.go`.

### Outputs

- Each issue independently stores zero or one optional challan string.
- Duplicate/reused challan numbers across products and recipient groups are accepted.
- Partial, case-insensitive return search finds every outstanding matching issue and
  leads into the existing safe holding/product return flow.
- Issue corrections and log filters/display preserve complete challan audit context.
- Old schema-1 files migrate safely to schema 2; issues without challans behave and
  marshal without a `challanNo` member.

### Side effects

- Successful issue/correction/return operations retain their one atomic save each.
- Challan typing/search, `/api` calls, log filtering, match selection and all stale or
  invalid paths perform no write.
- No challan record, counter, uniqueness index, automatic propagation or return field
  is stored.
- Store atomicity, `.bak` recovery, loopback binding and standard-library-only build
  remain unchanged.

## Files to create or modify

- `internal/register/model.go`, new `challan.go`, `log.go`, and tests.
- `internal/store/store.go` only if copying the expanded model requires it; tests must
  prove rollback of the new value.
- `internal/web/issue.go`, `return.go`, `corrections.go`, `log.go`, `views.go`,
  `server.go` and corresponding tests.
- `internal/web/templates/issue.html`, `return.html`, `edit.html`, `log.html`,
  `out.html`.
- `internal/web/static/return.js` only to enhance the same server-rendered controls;
  no-script behavior is mandatory.
- `specs/00-index.spec.md`.

## Required tests

`TestOldIssuesLoadWithEmptyChallan` — load pre-v1.1.1 schema-1 T3 JSON through the
spec 15 migration; every issue has empty challan, totals/holdings/log rows are
unchanged, in-memory/next-saved schema is 2, and empty challans remain omitted.

`TestFindOutstandingByChallanPartialCaseInsensitive` — create live issues `452`,
`CH-452`, `452-A`, `CH-999`, empty; queries `45` and `ch-452` return the exact expected
issue IDs in contract order with correct outstanding quantities and labels.

`TestFindOutstandingByChallanReturnsEveryProductAndGroup` — challan 452 on 10 cables,
20 chairs and 5 tables to Ravi/Amit plus 7 chairs to Meera returns four rows; quantities
remain `10,20,5,7`, not multiplied by recipient count.

`TestFindOutstandingByChallanSkipsSettledAndDeleted` — settled issue, tombstoned issue,
live issue under a deleted product and empty-challan issue are absent; a partially
returned live issue reports only its remaining quantity.

`TestChallanMatchCarriesSafeHoldingAnchor` — Ravi's solo Chairs issue and one joint
Ravi/Amit issue return respectively the solo holding's oldest anchor and the joint
issue ID; recipients are copied, not aliased.

`TestIssueFormShowsOptionalChallan` — exact label/hint/name/order, blank on GET, and no
uniqueness or required marker.

`TestIssueStoresCleanedOptionalChallan` — post 10 cables to Ravi/Amit with
`challanNo=  CH-452   A `; one issue stores `CH-452 A`, quantity 10 once, and success
status/banner remain spec 13's.

`TestIssueWithoutChallanRemainsCompatible` — existing spec 08/13 posts store empty,
omitempty removes the JSON member, and all exact existing success strings remain.

`TestIssueAllowsSameChallanForProductsAndGroups` — create the stakeholder example as
three issues: 10 cables, 20 chairs, 5 tables for Ravi/Amit, then 7 chairs for Meera,
all `452`; all four succeed and are independent. A fifth issue with another challan
also succeeds.

`TestIssueNeverReusesPreviousChallan` — after successful `452`, every new issue GET
has blank `challanNo`; product/recipient query parameters cannot prefill it.

`TestIssueRefusalKeepsOnlyPostedChallan` — over-issue with `CH-999` re-renders that
value; a later clean GET is blank and the failed value was never stored.

`TestFixIssueChallanAddsChangesAndRemoves` — empty → CH-452 → 452 → empty produces the
three exact `Change` values/phrases above in order, changes no stock/allocation, and
passes `Validate` after each save.

`TestOutAndReturnLinesShowChallanOnlyWhenPresent` — non-empty issue rows show
`Challan no. 452`; legacy empty rows have no placeholder or empty challan label.

`TestReturnSearchByChallanShowsEveryOutstandingMatch` — `GET /return/new?challan=45`
renders every matching stakeholder-example row with exact challan, product quantity,
full recipient label, issue date/time and server-rendered `Pick this holding` link.

`TestReturnSearchByChallanNoMatch` — exact empty sentence, 200 and byte-identical store.

`TestReturnChallanSearchTakesPrecedenceOverPersonQuery` — with both `q=Meera` and
`challan=452`, render only challan matches; choosing one removes `q`, and clearing
challan restores the existing Meera person-search flow.

`TestReturnChallanMatchSelectsExistingHoldingProduct` — tap the 20 Chairs match; the
next screen re-derives the holding, selects Chairs and prefills the complete outstanding
Chairs quantity for that holding without exposing an ID.

`TestReturnByChallanCannotCrossHoldingBoundary` — same challan on Ravi/Amit joint and
Meera solo Chairs; selecting group and returning 15 allocates only group, leaving 5
group and 7 Meera chairs.

`TestReturnByChallanRefusesStaleHolding` — settle/delete after results render; exact
stale banner, no return, valid register.

`TestLogCarriesInwardAndIssueChallans` — T1 inward STH/4471 and new issue CH-452 have
their exact `LogEntry.ChallanNo`; unrelated kinds remain empty.

`TestLogFilterByPartialChallan` — query `45` returns incoming/outgoing `452`, CH-452 and
452-A rows including their correction/deletion rows, case-insensitively; adding product,
person, day and kind filters intersects rather than widens the result.

`TestLogPageSearchesChallanAndKeepsFilters` — exact field/hint/reset/advice strings;
filter links retain `challan=45`; issue/inward rows show exact notes; blank challans
show none.

`TestChallanCorrectionUsesSharedChangePhrase` — `/out`, edit history and `/log` use the
three exact add/change/remove phrases from `corrections.go`; `log.go` contains no copy.

`TestIssueChallanSaveFailureRollsBack` — forced failure leaves the issue absent and
memory/disk byte-identical.

## Acceptance criteria

1. All required and pre-existing tests pass with `-race -count=1`.
2. Optional challans round-trip exactly after whitespace cleaning; empty stays absent
   from issue JSON; duplicate text is accepted without coupling records.
3. Return search is case-insensitive partial, excludes non-outstanding/deleted data,
   and displays every required field before selection.
4. Return POST still trusts only save-time holding derivation, `CheckReturn` and
   allocation; challan never selects an allocation directly.
5. Corrections, activity-log notes and activity-log filtering include challan with one
   shared correction phrase implementation.
6. No production type named `Challan`, challan slice, challan ID/counter, uniqueness
   check or auto-reuse state exists.
7. The combined `v1.1.1` browser scenario passes through the Browser skill's
   Playwright interface against a real native binary.
8. Standard-library-only Windows amd64 build succeeds with cgo disabled.

### Browser acceptance scenario

The release gate builds a native binary into a fresh temporary directory, starts it,
discovers its loopback URL from stdout, and always stops it. Through the Browser
skill's Playwright interface it performs, in order:

1. add Suresh Kumar, start his shift, and verify every visited page shows `Dashboard`
   and `Change person`;
2. add Chairs with 50 received, Charcoal sacks with 12, and Cable reels with 20 through
   the real product/inward flow;
3. on `/log`, type `ch`, observe Chairs and Charcoal sacks suggestions, choose Chairs,
   and verify the filter applies only after selection;
4. add/switch to Anita Rao from a non-dashboard page, return to Dashboard, and verify a
   later issue is attributed to Anita while the earlier inward remains Suresh's;
5. issue 10 Chairs jointly to Ravi Menon and Amit Sharma with challan `CH-452`, then 5
   Cable reels to the same people with the same challan;
6. rename Chairs to Folding chairs from Dashboard and verify Stock, Out and Who did
   what use the new name while the log shows old → new and Anita/time;
7. search returns with partial lowercase `452`, verify both products and full joint
   recipients appear, select Folding chairs and return all 10;
8. delete Folding chairs with the displayed fresh impact version and reason
   `This product should not be in the register.`, verify it and all related entries disappear
   from working screens/stock effects, then verify the deleted audit rows and reason in
   `Who did what`;
9. reload from the saved JSON and repeat the key absence/audit assertions, proving the
   behavior survives restart.

The browser run adds no repository file, Go module or production dependency. Record
its pass/fail result and any screenshots used for diagnosis in the release report.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestOldIssues|TestFindOutstandingByChallan|TestChallanMatch|TestLog.*Challan' -race -count=1 -v
go test ./internal/web/ -run 'TestIssue.*Challan|TestFixIssueChallan|TestOutAndReturnLinesShowChallan|TestReturn.*Challan|TestLog.*Challan|TestChallanCorrection' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'CheckIssue' internal/web/issue.go
rg -n 'JointHoldingForIssue|CheckReturn' internal/web/return.go
rg -n 'type Challan[[:space:]]|Challans +\[\]|NextID\("CH' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n 'unique.*challan|challan.*unique|lastChallan|previousChallan' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n 'net/http' internal/register internal/store # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. The outward-challan, challan-search and new log strings are not present in the
   approved walkthrough. They directly implement recorded stakeholder decisions and
   require `plain_language_reviewer` review; any rewrite must amend this contract and
   verbatim tests together.
