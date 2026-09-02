# Spec: Money Movements, Audit, and Transaction Journal

## Objective

Record every outgoing payment and incoming refund/proceed as an exact, chronological,
correctable but never silently erasable financial event, and provide a protected,
printable transaction journal with day/range filtering.

## Context

- User decisions recorded 2 September 2026: payments may happen before delivery, later
  in the day, or in several installments; one invoice may be entered broadly or split by
  deposit, rent, freight, unloading labour and miscellaneous purposes; purpose and mode
  are mandatory reusable suggestions; refunds and sale proceeds must be recordable;
  agreed order total is optional; corrections remain permanently audited.
- Spec 17 protects the vault/account identity. Spec 18 owns orders, parties, purposes,
  modes and exact INR parsing. Spec 20 owns physical supplier returns and sales. Money
  and stock events remain separate because they may happen at different times.

## Contract

### Inputs

Append to `FinanceData` immediately before `Audit`:

```go
Movements []MoneyMovement `json:"movements"`
```

Normalize it to `[]`.

#### Money movement and audit types

```go
type MoneyDirection string
const (
    MoneyOut MoneyDirection = "out"
    MoneyIn  MoneyDirection = "in"
)

type FinanceProductRef struct {
    ProductID   string `json:"productId"`
    ProductName string `json:"productName"` // snapshot at movement time
}

type MoneyMovement struct {
    ID          string              `json:"id"` // "MOV-0001"
    Direction   MoneyDirection      `json:"direction"`
    AmountPaise int64               `json:"amountPaise"` // > 0
    OccurredAt  time.Time           `json:"occurredAt"`
    PartyID     string              `json:"partyId"`
    OrderID     string              `json:"orderId,omitempty"`
    OrderLineIDs []string           `json:"orderLineIds,omitempty"`
    Products    []FinanceProductRef `json:"products,omitempty"`
    PurposeID   string              `json:"purposeId"`
    ModeID      string              `json:"modeId"`
    Reference   string              `json:"reference,omitempty"`
    Remarks     string              `json:"remarks,omitempty"`
    RecordedAt  time.Time           `json:"recordedAt"`
    RecordedByID string             `json:"recordedById"`
    Changes     []FinanceChange     `json:"changes,omitempty"`
    Voided      *FinanceVoid        `json:"voided,omitempty"`
}

type FinanceVoid struct {
    At          time.Time `json:"at"`
    ByAccountID string    `json:"byAccountId"`
    ByName      string    `json:"byName"`
    ByMobile    string    `json:"byMobile"`
    Reason      string    `json:"reason"`
}

```

`FinanceAuditEvent` is owned by spec 17. This spec adds exact kinds
`movement_created`, `movement_edited`, and `movement_voided`.

`RecordedAt`/`RecordedByID` never change. Actor snapshots in `FinanceChange`,
`FinanceVoid` and `FinanceAuditEvent` never resolve through the current account name;
they retain who acted at that time even if the account is later renamed/disabled.

Amounts use spec 18's rupee parser and formatter, never float. `OccurredAt` defaults to
now, accepts a local `datetime-local`, and may be backdated/future-dated;
`RecordedAt`/audit `At` use actual save time. Chronological views sort `OccurredAt`
descending, then ID descending; audit sorts actual `At` descending, then ID descending.
Every paid/received/net/order total uses checked int64 addition/subtraction; a create or
correction that would overflow any affected total is refused with
`That amount is too large to total safely.` and no write.

#### Recording movements

`GET /finance/movements/new` renders heading `Record money` and one repeatable row:

- `Money paid` / `Money received` (required direction);
- `Amount (₹)`;
- `Date and time`;
- mandatory typeaheads `Supplier or other party`, `Purpose`, `Payment mode`;
- optional `Related order` select;
- optional multi-select `Related products` populated from that order, or from live main
  products when there is no order;
- optional `Reference` and `Remarks`;
- buttons `Add another amount` and `Save transaction`.

One submitted row creates one `MoneyMovement`. Several rows submitted together are
validated completely and appended atomically, preserving form order for IDs. This is
how one ₹14,000 settlement may be entered as four rows—₹5,000 deposit, ₹5,000 rent,
₹2,000 freight, ₹2,000 unloading labour—with shared party/time/mode/reference copied by
the client but separately posted. It may instead be one ₹14,000 row with a broad custom
purpose. The server never automatically splits or combines amounts.

Party, purpose and mode resolve/create through spec 18 inside the same
`UpdateFinance`. A mode such as `Product adjustment` is valid and still requires a
positive agreed INR value plus remarks explaining what was adjusted. No mode implies
cash actually changed hands.

`OrderID` is optional. When present it must identify an existing order; every
`OrderLineID` must belong to it. `PartyID` may differ from the order supplier—for
example, freight may be paid to a transporter and unloading labour to a contractor.
Empty line IDs mean
the movement covers the whole order. When no order is present, `OrderLineIDs` must be
empty and selected live products are stored as snapshots. Invalid/duplicate IDs refuse
the complete submission. Product rename/deletion later never rewrites snapshots or
removes the movement.

`Products` always freezes what the transaction covered: with selected order lines it
copies those lines' product snapshots; with an order and no line IDs it copies all order
line snapshots in order while the UI labels the association `Whole order`; without an
order it copies the selected live catalogue names. A later order edit, product rename,
product tombstone or reuse of the same spelling never changes these movement snapshots.

Success redirects 303 to `/finance/journal?saved=<firstID>` with
`Transaction saved.` or `<n> transactions saved.`. Recording money never changes an
inward, on-hand quantity, supplier-return limit, sale quantity or order expected
quantity. Order summaries derive:

```text
Paid          = sum live MoneyOut linked to order
Received back = sum live MoneyIn linked to order
Net paid      = Paid - Received back
```

For an exact agreed total only, display `Remaining against agreed total` as
`AgreedPaise - NetPaid`; do not call it an amount owed. Estimated/blank totals show no
remaining figure.

#### Corrections and voids

`GET, POST /finance/movements/{id}/edit` supports every entered field. POST validates
the complete resulting row, changes current values and appends one `FinanceChange` per
changed field in form order: direction, amount, occurred time, party, order/products,
purpose, mode, reference, remarks. `From`/`To` are exactly rendered user values, with
money as `₹5,000.00`; actor ID/name/mobile and save time are captured. One protected
audit event is appended per edit summarizing the movement ID; field detail remains in
`Changes`.

Exact `Field`/`Label` pairs are: `direction`/`Money paid or received`,
`amount`/`Amount`, `occurredAt`/`Date and time`, `party`/`Supplier or other party`,
`order`/`Related order`, `products`/`Related products`, `purpose`/`Purpose`,
`mode`/`Payment mode`, `reference`/`Reference`, `remarks`/`Remarks`. Direction renders
`Money paid` or `Money received`; empty optional values render `Blank`; order/product
values use their stored historical display/snapshots. Identical cleaned submissions
append nothing and perform no save.

`POST /finance/movements/{id}/void` requires `Why are you voiding this transaction?`.
It sets `Voided` and appends audit; it never removes the row. A void movement is excluded
from all totals but remains in journal/audit marked `Voided — <reason>`. It cannot be
edited or voided again. To reverse a real payment/refund, record the opposite-direction
movement; void is only for an entry that should never have been recorded.

Example required behavior: order 100 Chairs, record ₹10,000.00 Money paid, goods never
arrive, mark order cancelled, then record ₹10,000.00 Money received with purpose
`Refund`. Both rows remain and net paid is ₹0.00. Deleting the original payment is not
an available action.

#### Ledger and printable transaction journal

`GET /finance` is the protected heading `Financial ledger` with actions `Record an
order`, `Record money`, `Return rented goods`, `Record a sale`, and summary tiles
`Money paid`, `Money received`, `Net paid`; tiles sum live movements across all dates.
Below, show the ten newest movement rows and links to `All transactions`, `Orders`, and
`Financial activity`.

`GET /finance/journal` heading is `Transaction journal`. It lists every movement,
including voided rows, with direction, exact amount, occurrence date/time, party,
purpose, mode, related order/products when present, reference, remarks, recorded-by
identity/time, correction lines and void line. No inventory on-duty identity is used.

GET filter parameters:

| Parameter | Meaning |
|---|---|
| `day=YYYY-MM-DD` | one local calendar day |
| `from=YYYY-MM-DD&to=YYYY-MM-DD` | inclusive local calendar range |
| `fromTime=YYYY-MM-DDTHH:MM&toTime=YYYY-MM-DDTHH:MM` | inclusive exact local date/time range |

No parameters means every transaction. A complete exact-time range takes precedence,
then `day`, then the date range. Exact time requires both `fromTime` and `toTime`, each
parsed in the server's local location, and includes movements where
`OccurredAt == start` or `OccurredAt == end`. A date range requires both ends and
`from <= to`; either range with only one end, parse failure, or end before start
re-renders 200 with `Choose a valid date or date range.` and no filtering/write.
Boundaries compare local `OccurredAt`, not `RecordedAt`, so a backdated payment prints
on the business day entered. Filter controls read `One day`, `From`, `To`, `Exact
start`, `Exact end`, `Show`, and `Every date`.

`GET /finance/journal/print` is a protected server-rendered print view accepting and
validating the identical parameters. Its rows are chronological ascending by
`OccurredAt`, then ID, even though the screen list is newest first. The
`Print this journal` link preserves the active filter query and points to this route;
its print-page button calls `window.print()`. All filtered content works without
JavaScript.

`app.css` adds `@media print`: hide chrome/navigation/actions/filter controls; retain
heading, active date/range, generated timestamp, rows, corrections and void marks;
use black text on white and do not truncate values. Print output must contain
`Transaction journal`, selected date/range and `Printed <longstamp>`.

`GET /finance/audit` heading `Financial activity` lists all `FinanceAuditEvent` rows,
including accounts, list maintenance, orders, money, returns and sales. It is visible to
every financial user; only admin controls are role-gated. Audit has no edit/delete route.

### Outputs

- Every real money movement remains chronologically visible; corrections and mistakes
  cannot erase the original record or actor.
- Refunds/proceeds are incoming rows; payments/expenses are outgoing rows; their timing
  is independent of orders and physical goods.
- The protected journal is useful on screen and on paper for one day or inclusive dates.

### Side effects

- Successful batch create/edit/void is one encrypted atomic save with audit.
- Journal/dashboard/audit/filter/print GETs perform no write.
- Void affects financial totals only; no money event affects stock.

## Files to create or modify

- `internal/register/finance_model.go`, `finance_ledger.go`, `finance_validate.go` and
  tests — movement/totals/filter/audit rules.
- `internal/store/finance.go` and tests — deep-copy/rollback of nested movement data.
- `internal/web/finance_ledger.go`, templates, `static/app.css`, optional print-only JS,
  and tests.

## Required tests

`TestMoneyMovementStoresExactPaiseAndAuthenticatedActor` — ₹5,000 deposit payment by
Asha stores 500000 paise, occurrence versus record time, immutable FAC/name/mobile
audit identity, and no stock/order-quantity change.

`TestSplitAndBroadPaymentsAreIndependentRows` — stakeholder ₹14,000 example entered as
four atomic rows totals ₹14,000; a separate broad row remains one event; IDs/order and
restart round-trip are exact.

`TestMovementReusableSuggestionsAreMandatory` — new Sharma Events, Unloading labour and
Online values are created with the movement and immediately suggested to Rohan; a
refusal cannot leave orphan suggestion values.

`TestMovementMayCoverWholeOrderOrSeveralProducts` — one payment with no line IDs covers
whole mixed order; another pays a transporter rather than the order supplier and
references Chairs and Tents lines successfully; unknown order/non-order line IDs refuse
without write.

`TestStandaloneMovementAndProductAdjustment` — standalone labour expense without order
or product succeeds; Product adjustment with Chairs/Cable reels and explanatory remarks
succeeds as exact valued row and causes no stock movement.

`TestCancelRefundKeepsBothDirections` — exact 100-chair/₹10,000 example retains paid,
incoming refund and cancellation, totals paid/received/net 10000/10000/0.

`TestMovementCorrectionKeepsEveryOriginalValue` — correct amount, purpose, mode, party,
products and backdated time; current row updates, ordered change entries/audit preserve
all old/new values and authenticated actor.

`TestVoidNeverDeletesAndOppositeMovementIsReversal` — void typing mistake excludes it
from totals but keeps journal/audit; real refund uses MoneyIn and leaves MoneyOut live;
no route/symbol physically deletes movements.

`TestJournalChronologicalAndDateFilters` — movements at 31 Aug 23:59, 1 Sep 00:00 and 2
Sep 18:00 local; exact single-day and inclusive range results/order; day precedence;
invalid/partial/reversed ranges show exact refusal and byte-identical store.

`TestJournalExactTimeRangeIsInclusive` — movements on 1 January 2016 at 22:59, 23:00,
23:20, 23:39 and 23:40 local; `fromTime=2016-01-01T23:00&toTime=2016-01-01T23:39`
returns exactly the 23:00/23:20/23:39 rows. Exact range takes precedence over supplied
day/date parameters; partial, invalid and reversed exact ranges refuse without write.

`TestJournalIsProtectedAndPrintableWithoutJavaScript` — public redirect/no leakage;
authenticated server HTML contains full rows/filter links and print CSS; script-disabled
submission still filters; `/finance/journal/print` preserves the filter, sorts ascending,
contains heading/range/printed stamp and has no chrome/actions.

`TestFinancialAuditVisibleButImmutable` — user and admin see actor snapshots for account,
list, order and movement events; no audit mutation route exists and hand-built POST is
405/no write.

`TestConcurrentMovementPostsKeepAllRowsAndTotals` — 20 concurrent ₹500.00 freight rows
produce 20 unique records and ₹10,000.00 paid, valid/decryptable after restart.

## Acceptance criteria

1. All money values are positive int64 paise and all formatting/parsing avoids float.
2. Every create/edit/void has immutable authenticated identity and append-only audit;
   no financial event is physically deleted.
3. Money totals include only live movements and have no stock side effect.
4. Journal filters use occurrence local dates, are server-rendered, inclusive and
   printable with corrections/voids visible.
5. Protected values do not appear in public responses or plaintext storage.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestMoneyMovement|TestSplit|TestMovement|TestCancelRefund|TestVoid|TestJournal|TestConcurrentMovement' -race -count=1 -v
go test ./internal/web/ -run 'TestMoneyMovement|TestSplit|TestMovement|TestCancelRefund|TestVoid|TestJournal|TestFinancialAudit|TestConcurrentMovement' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'float(32|64)|ParseFloat|FormatFloat' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n 'Movements = (append\([^)]*\[:|nil)|delete\(.*Movement' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n '@media print' internal/web/static/app.css
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. Currency is INR only in this release. The user discussed rupees and did not request
   multi-currency conversion; adding it later requires a new explicit spec.
2. A custom mode records how value was settled; `Product adjustment` does not itself
   create a physical inventory movement. The related stock sale/return must be recorded
   separately under spec 20 when applicable.
