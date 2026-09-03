# Spec: Financial Orders and Reusable Suggestions

## Objective

Let an authorized person record an order before goods arrive, including several
products and an acquisition basis per line. Use one public supplier/other-party list on
the unauthenticated inward desk and authenticated finance screens, while keeping purpose
and payment-mode suggestions encrypted.

## Context

- User decisions recorded 2 September 2026: one order may contain several products;
  intended quantities do not control what inventory staff record as physically received;
  agreed total is optional and may be estimated or exact; new order products must appear
  in the ordinary inventory picker at zero stock; suppliers, purposes and payment modes
  are reusable suggestions; initial modes are Cash, UPI, Bank transfer, Cheque and Card;
  custom values such as Online and Product adjustment are allowed.
- User decision recorded 3 September 2026: supplier/payee names are shared plain data
  because the unauthenticated inward person must use the same names. This exposure is
  limited to stable IDs, current names and old-name/merge aliases; the money portion
  remains protected.
- Spec 17 owns the public `Register.Parties` boundary, encrypted vault, financial
  identity, migration and admin authorization. This spec never exposes a purpose, mode
  or financial relationship to an ordinary route or API.
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

Schema migration and a fresh vault normalize both to `[]`; schema-5 migration removes
old party-kind rows as specified in spec 17 while preserving purpose/mode rows.
This spec adds audit kinds `order_created`, `order_edited`, `order_cancelled`,
`value_created`, `value_renamed`, `value_merged`, and `value_deleted`.

#### Reusable values

```go
type FinanceValueKind string
const (
    FinancePurpose FinanceValueKind = "purpose"
    FinanceMode    FinanceValueKind = "mode"
)

type FinanceReusableValue struct {
    ID          string           `json:"id"` // "PUR-0001" | "PMD-0001"
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

`FinanceParty == "party"` and old `PTY-*` reusable values may remain as a schema-4
read/migration compatibility type only; schema 5 creates no protected party value and
persists none after the first successful authenticated finance write. `CleanName` is
applied before storage; blank is refused. `FoldKey` is unique among unmerged values of
the same kind. Prefixes are fixed as above. At first finance setup,
create the five mode values in this order: `Cash`, `UPI`, `Bank transfer`, `Cheque`,
`Card`, attributed to FAC-0001 at setup time. No initial party or purpose is created.

Every party, purpose and mode control is a mandatory typeahead. Party controls use the
public `Register.Parties` model and `GET /api/parties?q=<text>`; purpose and mode controls
use protected `GET /finance/api/values?kind=purpose|mode&q=<text>`. Each returns JSON
objects containing exactly the suggestion ID, value and label needed by the picker.
Matching and form behavior are:

- case-insensitive substring match after `CleanName`/`FoldKey`;
- values beginning with the query first, then other substring matches, alphabetically
  by folded value in each group, capped at eight;
- selecting a suggestion submits its hidden ID;
- changing visible text clears that ID;
- if no exact folded match exists, a final row reads `+ Add “<cleaned text>”` and the
  POST creates that value in the same atomic transaction as the order/movement;
- an exact folded match must resolve to its existing ID and never creates a duplicate;
- the server repeats party resolution/creation against `Register.Parties` and protected
  value resolution/creation inside `Store.UpdateFinance`; client behavior is only an
  enhancement.

The no-script fallback is an existing-value `<select>` plus `Or add a new one` text
field. Fields are `partyIdChoice`/`partyNameNew`,
`purposeIdChoice`/`purposeNameNew` and `modeIdChoice`/`modeNameNew`. A nonblank typed-new
field wins over its select; otherwise the selected ID is used. If neither is
valid/nonblank, refuse. The JS fields remain hidden `partyId` plus visible `partyName`,
and the corresponding purpose/mode names.

`GET /api/parties` is available to an ordinary on-duty inventory request without
financial authentication and to a confirmed authenticated finance session even before
an inventory shift begins. The public response contains party IDs and current names
only. In the latter case it uses `Store.ReadBoth` so unmigrated schema-4 vault parties
are also suggested before the first schema-5 write. It never returns amount, purpose,
mode, account/mobile, timestamps, audit provenance or a record using the party.

Admin routes `GET /finance/lists` and `POST /finance/lists/{id}/rename`, `/merge`, and
`/delete` manage public parties and protected purposes/modes. A purpose/mode rename
appends the existing encrypted `FinanceChange`; a party rename changes public `Name`,
appends the prior wording to public `PreviousNames`, and appends encrypted audit kind
`party_renamed` with label `Supplier or other party corrected`. It does not put actor or
time into the public row. A party combine sets public `MergedIntoID`, keeps every inward
and finance record's stored ID unchanged, resolves all screens through the target and
appends encrypted `party_merged` audit. Cycles and same-target combines are invalid.

A party may be physically deleted only when no inward, order, money movement, supplier
return, sale or other party merge points at it. Deleted unused parties append encrypted
`party_deleted` audit. Exact party refusal: `This name has been used. Rename it or merge
it instead.` Protected purpose/mode deletion retains the existing exact refusal `This
value has been used. Rename it or merge it instead.` Historical audit text alone does
not make a value used.

The public-party store operations are exactly:

```go
func (s *Store) RenameParty(vaultKey []byte, actorID, partyID, newText string, now time.Time) error
func (s *Store) MergeParty(vaultKey []byte, actorID, partyID, targetID string, now time.Time) error
func (s *Store) DeleteParty(vaultKey []byte, actorID, partyID string, now time.Time) error
```

Each requires an active administrator and one `UpdateFinance` transaction. Its encrypted
audit event has `EntityType: "party"`, the source `EntityID`, immutable actor ID/name/
mobile/time, and these exact values:

| Action | Kind | Summary | Before | After |
|---|---|---|---|---|
| rename | `party_renamed` | `Supplier or other party corrected` | old current name | new current name |
| combine | `party_merged` | `Supplier or other party merged` | source name | target name |
| delete unused | `party_deleted` | `Supplier or other party removed` | deleted name | blank |

List controls say `Rename`, `Combine duplicates`, and `Delete unused value`; never the
database term `Merge` by itself. The page introduction says `Supplier names and ways
goods came in also appear on delivery screens; purposes and payment modes appear only
in financial details. Renaming an entry changes current records, while Financial
activity keeps the earlier wording.` A used row says `In use. Rename it or combine it
with another entry.` Combine and delete are two-step. The first POST writes nothing and
renders:

- for a party, `Combine <source> into <target>?` / `Every delivery and financial record
  that currently shows <source> will show <target>. The activity history will keep both
  earlier names.` / button `Yes, combine these values`;
- for a protected purpose/mode, `Combine <source> into <target>?` / `Every financial
  record that currently shows <source> will show <target>. The activity history will
  keep both earlier names.` / button `Yes, combine these values`;
- for a party, `Delete unused supplier or other party “<value>”?` / `It will be removed
  from future suggestions. This is allowed only because no delivery and no financial
  record uses it.` / button `Yes, delete this unused value`;
- `Delete unused <kind> “<value>”?` / `It will be removed from future suggestions. This
  is allowed only because no current financial record uses it.` / button `Yes, delete
  this unused value`.

Only a second CSRF-protected POST with `confirm=yes` mutates. Both values, same-kind,
use status and cycle/uniqueness rules are rechecked inside `UpdateFinance`; a changed
target re-renders fresh impact and writes nothing.

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

Successful POST re-resolves the party against public `Register.Parties` and products,
creates any new public party, validates at least one line and every positive
quantity/basis, then appends one encrypted order in one `UpdateFinance`. It redirects
303 to `/finance/orders/{id}` and says `Order saved.` Creating an order may add only the
party's public ID/name row; it changes no inward, on-hand, supplier obligation or money
balance and exposes no order-to-party link outside the vault.

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
Cancellation uses a two-step form. Before the reason/action, show `Cancel this order?`
and `The order will stay in the ledger. Existing payments, receipts and stock entries
will not change. Record any refund separately.` Button: `Yes, cancel this order`.
The order detail's first-step control is `Review cancellation` and contains no reason
field; the reason is requested only after the consequence warning.
The first POST writes nothing; the second CSRF-protected POST carries `confirm=yes`,
requires the reason and rechecks that the order is still open before saving.
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

The ordinary inward and correction forms use the same party picker as finance, labelled
`Came from`. Their fields are `partyId`/`partyName` with no-script
`partyIdChoice`/`partyNameNew`. A valid selection stores its resolved ID in
`Inward.PartyID`; a new cleaned name is added to `Register.Parties` in the same ordinary
atomic write. `Inward.Supplier` stores the name snapshot used at save time. A blank party
remains allowed. For a stale pre-schema-5 browser form, POST `/inward/new` and inward
correction accept old field `supplier` only when all new party fields are blank, then
resolve/add it exactly like typed `partyName`; this compatibility must not override a
new-form selection.

### Outputs

- Orders capture intent and optional cost without changing physical stock.
- New products are deliberately shared with inventory at zero stock. Party names/IDs and
  aliases are deliberately shared with inventory; every amount, purpose, mode and
  financial link remains protected.
- Every typed party becomes an immediate suggestion to inventory and finance; every
  typed purpose/mode becomes an immediate suggestion to financial users. Admin changes
  are audited inside the vault.

### Side effects

- Order/product/party/reusable-value writes are one atomic save with authenticated audit
  identity encrypted; only the contracted public product/party fields are plaintext.
- Typeahead queries and every GET are read-only. Purpose/mode queries require a finance
  session; party queries serve either an ordinary on-duty request or a confirmed finance
  session.
- Cancel/edit/list maintenance never erases order or ledger history.

## Files to create or modify

- `internal/register/finance_model.go`, `finance_order.go`, `finance_validate.go` and
  tests — order/value model, resolution and validation.
- `internal/register/model.go`, `parties.go` and tests — public party model, resolution,
  aliases, cross-boundary references and legacy inward linking.
- `internal/store/finance.go` and tests — copying/encrypted transactional updates.
- `internal/store/parties.go` and tests — schema-4 vault import and audited admin actions.
- `internal/web/finance_orders.go`, `finance_values.go`, `parties.go`, `inward.go`,
  correction handlers, templates/static enhancement and tests.
- `internal/web/products.go` only to share existing product guard helpers without
  weakening the ordinary route.

## Required tests

`TestInitialPaymentModesAndMandatorySuggestions` — fresh setup creates the five exact
modes/order; typing `ba` suggests Bank transfer; creating Online makes it immediately
available to another finance user; exact/case-fold reuse creates no duplicate.

`TestFinanceSuggestionOrderingSelectionAndNoScript` — prefix-before-substring ordering,
eight cap, hidden ID clearing, add row, and equivalent server-only selection/new-value
flow for Sharma Events, Freight and Product adjustment.

`TestSharedPartySuggestionsWorkAtDeskAndBeforeFirstFinanceWrite` — legacy encrypted
Sharma Events is returned by authenticated `/api/parties?q=sha` before an inventory
shift and before any schema-5 write; after migration the on-duty inward form returns the
same ID/name without decrypting finance; neither response contains amount, purpose,
mode, actor, mobile, timestamp or financial link.

`TestInwardPartyPickerAndStaleSupplierFormUseOneList` — Suresh records 70 Tents from a
selected Sharma Events and later 30 through a stale `supplier=Sharma Events` form; both
inwards keep their supplier snapshot, point at one shared party ID and Stock reaches
100. A stale inward-correction POST also resolves its `supplier` field into the shared
list. A simultaneous new-form selection plus stale supplier field uses the new fields.

`TestAdminRenamesMergesAndDeletesReusableTypos` — rename Frieght to Freight; merge Online
payment into Bank transfer; physically delete unused Sharm Events; audit has old/new,
admin ID/name/mobile/time; used deletion gets the applicable exact refusal and no bytes
change.

`TestAdminPartyRenameMergeAndDeleteKeepPublicRowsMinimal` — rename Sharma Events to
Sharma Event Hire, combine Sharm Events into it and delete unused Bala Transprt. Existing
inward/order/movement/return/sale IDs do not change and resolve to the kept name; public
JSON contains only `id`, `name`, `previousNames`, `mergedIntoId`; encrypted audit retains
admin ID/name/mobile/time; deleting a used party gets `ErrPartyUsed` and no write.

`TestReusableListCombineAndDeleteRequireImpactConfirmation` — first POST shows the exact
plain-language target/consequence/action and writes nothing; only `confirm=yes` combines
or deletes, with all save-time use/kind/target guards and audit intact.

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
ordinary pages contain no order/payment fields; the inward picker contains only the
contracted public party name/ID and reveals no financial relationship.

`TestCancelPaidUndeliveredOrderKeepsHistory` — cancel 100 Chairs after a movement from
spec 19: order and original payment remain, no product/inward is deleted, and a later
incoming refund may link to the cancelled order.

`TestOrderCancellationWarnsBeforeSecondConfirmation` — first POST shows the exact
keep-history/no-side-effect/refund warning and writes nothing; second POST requires
reason, CSRF and `confirm=yes`, then audits cancellation without changing money/stock.

`TestOrderCorrectionIsAuditedAndUsedLineCannotDisappear` — correction preserves
CreatedAt/By and snapshots; removing a movement-linked line gets exact refusal.

`TestOrderConcurrentNewValueResolutionDoesNotDuplicate` — two posts creating Sharma
Events at once result in one case-fold public party and two valid orders or one clean
refusal; public and vault references validate and decrypt after restart.

## Acceptance criteria

1. Every order has one party, at least one unique live product line, positive expected
   quantities and per-line rent/purchase; optional money is int64 paise.
2. Orders and their corrections/cancellation have no stock side effect.
3. Party/purpose/mode typeahead creation and reuse are mandatory with both JS and
   no-script paths; the party vocabulary is shared by desk and finance while
   purpose/mode stay encrypted; list management is admin-only and encrypted-audited.
4. Only the two deliberate product routes append main products; both apply identical
   duplicate/confirmation/save-time checks.
5. Ordinary routes expose product names and contracted party names/IDs/aliases, but no
   amount, purpose, mode, financial relationship, account or audit provenance.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestInitialPayment|TestFinanceSuggestion|TestAdminRenames|TestOrder|TestValidateParty|TestLinkInwardParties' -race -count=1 -v
go test ./internal/store/ -run 'TestImportVaultPart|TestFirstFinanceWrite|TestParty' -race -count=1 -v
go test ./internal/web/ -run 'TestInitialPayment|TestFinanceSuggestion|TestAdminRenames|TestReusableList|TestFinancialUser|TestOrder|TestFinanceOrder|TestFinanceProduct|TestInventoryReceives|TestCancelPaid|TestInward' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'Products = append' internal --glob '*.go' --glob '!**/*_test.go' # only ordinary and finance product-create owning functions
rg -n 'float(32|64)|ParseFloat|FormatFloat' internal/register internal/web internal/store # must print nothing
rg -n 'FinancePurpose|FinanceMode|amountPaise|purposeId|modeId' internal/web/inward.go internal/web/templates/inward.html internal/web/parties.go # must print nothing
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
