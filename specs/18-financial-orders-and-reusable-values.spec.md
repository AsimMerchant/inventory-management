# Spec: Financial Orders and Reusable Suggestions

## Objective

Let an authorized person record an order before goods arrive, including several
products and rent/purchase per line, and make every reusable typed party, purpose and
payment mode immediately available as a shared typeahead suggestion.

## Context

- User decisions recorded 2 September 2026: one order may contain several products;
  intended quantities do not control what inventory staff record as physically received;
  agreed total is optional and may be estimated or exact; new order products must appear
  in the ordinary inventory picker at zero stock; suppliers, purposes and payment modes
  are reusable suggestions; initial modes are Cash, UPI, Bank transfer, Cheque and Card;
  custom values such as Online and Product adjustment are allowed.
- Spec 17 owns the encrypted vault, financial identity and admin authorization. This
  spec never exposes its decrypted values to an ordinary route or API.
- Specs 06/15 own product duplicate, near-duplicate, rename and tombstone behavior.
  This spec adds a second deliberate product-creation route but preserves those guards.
- An order is intent, not inventory. No order quantity enters stock arithmetic and an
  inward never references or selects an order.

## Contract

### Inputs

Append these slices to `FinanceData` immediately before `Audit`:

```go
Orders         []FinanceOrder         `json:"orders"`
ReusableValues []FinanceReusableValue `json:"reusableValues"`
```

Schema-1/2 migration and a fresh vault normalize both to `[]`.
This spec adds audit kinds `order_created`, `order_edited`, `order_cancelled`,
`value_created`, `value_renamed`, `value_merged`, and `value_deleted`.

#### Reusable values

```go
type FinanceValueKind string
const (
    FinanceParty   FinanceValueKind = "party"
    FinancePurpose FinanceValueKind = "purpose"
    FinanceMode    FinanceValueKind = "mode"
)

type FinanceReusableValue struct {
    ID          string           `json:"id"` // "PTY-0001" | "PUR-0001" | "PMD-0001"
    Kind        FinanceValueKind `json:"kind"`
    Value       string           `json:"value"`
    CreatedAt   time.Time        `json:"createdAt"`
    CreatedByID string           `json:"createdById"`
    Changes     []FinanceChange  `json:"changes,omitempty"`
    MergedIntoID string          `json:"mergedIntoId,omitempty"`
}

type FinanceChange struct {
    At          time.Time `json:"at"`
    ByAccountID string    `json:"byAccountId"`
    ByName      string    `json:"byName"`
    ByMobile    string    `json:"byMobile"`
    Field       string    `json:"field"`
    Label       string    `json:"label"`
    From        string    `json:"from"`
    To          string    `json:"to"`
}
```

`CleanName` is applied before storage; blank is refused. `FoldKey` is unique among
unmerged values of the same kind. Prefixes are fixed as above. At first finance setup,
create the five mode values in this order: `Cash`, `UPI`, `Bank transfer`, `Cheque`,
`Card`, attributed to FAC-0001 at setup time. No initial party or purpose is created.

Every party, purpose and mode control is a mandatory typeahead:

- case-insensitive substring match after `CleanName`/`FoldKey`;
- values beginning with the query first, then other substring matches, alphabetically
  by folded value in each group, capped at eight;
- selecting a suggestion submits its hidden ID;
- changing visible text clears that ID;
- if no exact folded match exists, a final row reads `+ Add “<cleaned text>”` and the
  POST creates that value in the same atomic transaction as the order/movement;
- an exact folded match must resolve to its existing ID and never creates a duplicate;
- the server repeats resolution/creation inside `Store.UpdateFinance`; client behavior
  is only an enhancement.

The no-script fallback is an existing-value `<select>` plus `Or add a new one` text
field. If both are submitted, selected ID wins; if neither is valid/nonblank, refuse.

Admin routes `GET /finance/lists` and `POST /finance/lists/{id}/rename`, `/merge`, and
`/delete` manage these values. Rename requires a unique cleaned value and appends a
`FinanceChange` (`Field: "value"`, labels `Party`, `Purpose`, or `Payment mode`). Merge
requires a different live target of the same kind, sets `MergedIntoID`, appends audit,
and makes every resolver follow the target transitively; cycles are invalid. Existing
records are not rewritten and display the final target value. Delete is accepted only
when no current order, movement or settlement field references the value; historical
audit/change text does not make it used. It physically removes the unused typo but
appends a `FinanceAuditEvent` containing its kind/value.
Used values are never physically erased. Exact refusal:
`This value has been used. Rename it or merge it instead.`

#### Orders

```go
type FinanceOrder struct {
    ID            string             `json:"id"` // "ORD-0001"
    PartyID       string             `json:"partyId"`
    Lines         []FinanceOrderLine `json:"lines"` // at least one
    AgreedPaise   *int64             `json:"agreedPaise,omitempty"`
    AgreedKind    string             `json:"agreedKind,omitempty"` // "estimated" | "exact"
    OrderedAt     time.Time          `json:"orderedAt"`
    Status        string             `json:"status"` // "open" | "cancelled"
    Remarks       string             `json:"remarks,omitempty"`
    CreatedAt     time.Time          `json:"createdAt"`
    CreatedByID   string             `json:"createdById"`
    Changes       []FinanceChange    `json:"changes,omitempty"`
}

type FinanceOrderLine struct {
    ID                  string `json:"id"` // "OLN-0001", unique across vault
    ProductID           string `json:"productId"`
    ProductNameSnapshot string `json:"productNameSnapshot"`
    ExpectedQuantity    int    `json:"expectedQuantity"` // >= 1
    Basis               Basis  `json:"basis"` // rent | purchase
}
```

`GET /finance/orders/new` renders heading `Record an order` with `Supplier or other
party`, repeatable `Products ordered`, per-line `Product`, `Expected quantity`, and
`Rent or purchase`; `Order date and time`; optional `Agreed total`; choices `Estimate`
and `Exact`; optional `Remarks`; button `Save order`.

Amounts are INR. Parse user input as rupees with zero, one or two decimal places into
positive `int64` paise; commas, signs, exponents, more than two decimals, zero and
overflow are refused with `Type the agreed total in rupees, using up to two decimal
places.` If total is blank, `AgreedPaise == nil` and `AgreedKind == ""`; if present,
one kind is required. Rendering uses `₹` and exactly two decimals without float math.

Each product line uses the select-only product picker. It may select any live product,
or deliberately add a new product through `POST /finance/product/new` with the same
blank, folded-duplicate and four-character near-duplicate confirmation rules as specs
06/15. A successful creation appends to the main `Register.Products` inside
`Store.UpdateFinance` with zero stock, current authenticated display name in
`Product.CreatedBy`, and appends protected finance audit with immutable account ID and
mobile. It redirects back to the order draft with the new product selected. No other
finance handler may create products.

An order may repeat neither line ID nor the pair `(ProductID, Basis)`. The same product
may appear once on rent and once as a purchase in one order, but two rent lines for the
same product are refused. `OrderedAt` defaults to now and accepts a local
`datetime-local` value interpreted in the server's local offset; it may be backdated or
future-dated. `CreatedAt` is actual save time and never editable.

Successful POST re-resolves the party and products, creates any new party, validates at
least one line and every positive quantity/basis, then appends one order in one
`UpdateFinance`. It redirects 303 to `/finance/orders/{id}` and says `Order saved.`
Creating an order changes no inward, on-hand, supplier obligation or money balance.

`GET /finance/orders` lists newest `OrderedAt` first, ID tie-break, with status, party,
products/expected quantities/bases, optional `Estimated total` or `Agreed total`.
`GET /finance/orders/{id}` gives the same detail. Spec 19 later adds derived
`Paid`/`Received back` totals and record-money links; this spec neither declares nor
reads a future movement type. Do not label an estimated balance as due.

Financial displays always render `ProductNameSnapshot`; if the live catalogue resolves
the same ID under another name, append `now called <name>`. If the product is deleted or
its old name is later reused by a new product ID, the snapshot remains the primary
historical label and is never rewritten.

`POST /finance/orders/{id}/cancel` requires `Why is this order cancelled?`; sets status
`cancelled` and appends audit. It never voids payments, deletes products or changes
stock. A cancelled order remains visible and can receive a separately recorded refund.
Order field corrections use `/finance/orders/{id}/edit`, append one `FinanceChange` per
changed field in form order and retain line/product snapshots. Product-line removal is
allowed only if no movement references that line; otherwise exact refusal
`This product is already used by a ledger entry.` No financial record is silently
deleted.

The edit form contains the create fields. It may add a line; remove a line; or change a
line's product, basis or expected quantity. A line referenced by a movement may change
expected quantity only—removing it or changing product/basis gets the refusal above.
Changing an unreferenced line's product replaces its current `ProductNameSnapshot` with
the selected product's current name; the previous line text remains in `FinanceChange`.
Append changes in this exact order/table:

| Field | Label | From/To rendering |
|---|---|---|
| `party` | `Supplier or other party` | resolved value |
| `products` | `Products ordered` | each line as `<quantity> <snapshot> — rent|purchase`, joined `; ` |
| `agreedTotal` | `Agreed total` | `Not entered` or formatted ₹ amount |
| `agreedKind` | `Estimate or exact` | `Not entered`, `Estimate`, `Exact` |
| `orderedAt` | `Order date and time` | `2 January 2006 · 3:04 pm` |
| `remarks` | `Remarks` | exact cleaned text or `Blank` |

Submitting identical cleaned values is a no-op. Every successful non-no-op edit appends
one `order_edited` audit event after its field changes.

#### Separation from inward receipt

Finance-created products immediately appear in `/api/products`, Stock at zero, and the
ordinary `/inward/new` picker. The inventory person records only product, actual
quantity, date, basis, supplier/challan and on-duty receiver exactly as spec 07. They do
not see expected quantity, order party, total, payment or order ID and do not select an
order. Receiving 70 of an expected 100 and later 30 is two independent inwards; neither
changes the order or creates money movement.

Finance party suggestions are protected and must not populate the ordinary inward
supplier datalist. Existing inward supplier suggestions continue to come only from live
inwards. Case-fold comparison may later correlate an inward supplier to a finance party
for supplier-return limits in spec 20; it does not expose the protected list.

### Outputs

- Orders capture intent and optional cost without changing physical stock.
- New products are deliberately shared with inventory at zero stock; all other financial
  details remain protected.
- Every reusable typed value becomes an immediate shared suggestion for all financial
  users and can be safely corrected by an admin.

### Side effects

- Order/product/reusable-value writes are one encrypted atomic save with authenticated
  audit identity.
- Typeahead queries and every GET are read-only and require a finance session.
- Cancel/edit/list maintenance never erases order or ledger history.

## Files to create or modify

- `internal/register/finance_model.go`, `finance_order.go`, `finance_validate.go` and
  tests — order/value model, resolution and validation.
- `internal/store/finance.go` and tests — copying/encrypted transactional updates.
- `internal/web/finance_orders.go`, `finance_values.go`, templates/static enhancement
  and tests.
- `internal/web/products.go` only to share existing product guard helpers without
  weakening the ordinary route.

## Required tests

`TestInitialPaymentModesAndMandatorySuggestions` — fresh setup creates the five exact
modes/order; typing `ba` suggests Bank transfer; creating Online makes it immediately
available to another finance user; exact/case-fold reuse creates no duplicate.

`TestFinanceSuggestionOrderingSelectionAndNoScript` — prefix-before-substring ordering,
eight cap, hidden ID clearing, add row, and equivalent server-only selection/new-value
flow for Sharma Events, Freight and Product adjustment.

`TestAdminRenamesMergesAndDeletesReusableTypos` — rename Frieght to Freight; merge Online
payment into Bank transfer; physically delete unused Sharm Events; audit has old/new,
admin ID/name/mobile/time; used deletion gets the exact refusal and no bytes change.

`TestFinancialUserCannotManageReusableValues` — user may create values as part of a
record and reuse suggestions but every list-management POST returns 403/no write.

`TestOrderWithMultipleProductsAndMixedBasis` — Asha records 100 rented Chairs and 50
purchased Cable reels from Sharma Events, estimated ₹25,000.00; exact types/snapshots,
one party, no inward/movement/stock change, and encrypted restart round-trip.

`TestOrderAgreedTotalOptionalEstimatedOrExact` — blank, estimated and exact totals;
rupee parser accepts `5000`, `5000.5`, `5000.50` and refuses invalid/zero/overflow without
float arithmetic.

`TestFinanceOrderCanDeliberatelyCreateInventoryProduct` — create Tents through the
finance confirmation route; it records authenticated Asha identity, appears at zero on
Stock and in ordinary inward picker, and protected supplier/amount remain absent.

`TestFinanceProductCreationKeepsDuplicateGuards` — Chairs/chairs duplicate and a
four-character near duplicate require the same exact refusals/confirmation as spec 06;
hand-built posts cannot bypass recheck inside `UpdateFinance`.

`TestInventoryReceivesPartialOrderWithoutOrderKnowledge` — order 100 Tents; ordinary
desk receives 70 then 30; on-hand becomes 100, order remains expected 100/open, and all
ordinary pages contain no order/payment fields or finance party suggestions.

`TestCancelPaidUndeliveredOrderKeepsHistory` — cancel 100 Chairs after a movement from
spec 19: order and original payment remain, no product/inward is deleted, and a later
incoming refund may link to the cancelled order.

`TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear` — correction preserves
CreatedAt/By and snapshots; removing a movement-linked line gets exact refusal.

`TestOrderConcurrentNewValueResolutionDoesNotDuplicate` — two posts creating Sharma
Events at once result in one case-fold party and two valid orders or one clean refusal;
vault validates and decrypts after restart.

## Acceptance criteria

1. Every order has one party, at least one unique live product line, positive expected
   quantities and per-line rent/purchase; optional money is int64 paise.
2. Orders and their corrections/cancellation have no stock side effect.
3. Party/purpose/mode typeahead creation and reuse are mandatory with both JS and
   no-script paths; list management is admin-only and audited.
4. Only the two deliberate product routes append main products; both apply identical
   duplicate/confirmation/save-time checks.
5. Ordinary routes expose the new product name but no encrypted order/value detail.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestInitialPayment|TestFinanceSuggestion|TestAdminRenames|TestOrder' -race -count=1 -v
go test ./internal/web/ -run 'TestInitialPayment|TestFinanceSuggestion|TestAdminRenames|TestFinancialUser|TestOrder|TestFinanceOrder|TestFinanceProduct|TestInventoryReceives|TestCancelPaid' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'Products = append' internal --glob '*.go' --glob '!**/*_test.go' # only ordinary and finance product-create owning functions
rg -n 'float(32|64)|ParseFloat|FormatFloat' internal/register internal/web internal/store # must print nothing
rg -n 'ReusableValues|FinanceParty|FinancePurpose|FinanceMode' internal/web/inward.go internal/web/templates/inward.html # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Defects found while implementing

1. Two required tests, `TestCancelPaidUndeliveredOrderKeepsHistory` and
   `TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear`, depend on a money movement
   pointing at an order line, while the Contract above forbids this spec from declaring
   a movement type. Both are therefore **partial**: the refusal path and its exact
   wording `This product is already used by a ledger entry.` ship here behind
   `register.FinanceLineIsReferenced`, which returns `false` until spec 19 fills it in.
   Spec 19 must implement that predicate and complete both tests.
2. Money rendering. This spec says only "`₹` and exactly two decimals"; specs 19 and 21
   write every amount with digit separators (`₹5,000.00`, `₹25,000.00`). Since two
   later reviewed specs are explicit and this one is silent, `FormatRupees` groups
   digits the Indian way — three, then twos — so ₹1,23,45,678.90. This spec is the one
   that is under-specified, not the others.
3. The float grep in the verification block below has no `--glob '!**/*_test.go'`
   exclusion, unlike the `Products = append` grep on the line above it. A float literal
   in a test therefore fails the gate. The tree is float-free including tests, so the
   grep is left exactly as written rather than loosened.

## Open

1. Orders intentionally do not reconcile expected versus received quantities because
   the user explicitly said inventory staff may not know whether another delivery is
   coming and must simply record what physically arrived.
2. The UI strings in this spec extend beyond the approved walkthrough and require the
   plain-language reviewer before release.
