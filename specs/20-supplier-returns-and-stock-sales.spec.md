# Spec: Supplier Returns and Stock Sales

## Objective

Capture the two end-of-event physical exits—rented goods returned to a supplier and
purchased goods sold—without confusing either with money received, while keeping public
on-hand stock correct and preserving protected financial history.

## Context

- User decisions recorded 2 September 2026: after the event, rented goods are returned
  and deposits may be received back; purchased goods may be sold and proceeds received;
  all must be captured. Money may arrive before/after/in installments, so physical and
  money movements are distinct.
- Existing stock is pooled per product and inwards are not lots for issue/return. The
  user reaffirmed that inventory staff simply record actual receipts, including partial
  deliveries, without choosing an order.
- **Superseded 3 September 2026 by the user.** The original decision tied a supplier
  return to the quantity that party itself sent. The user rejected it in use: goods are
  handed to a transporter, or to whoever is doing the rounds, and that person then
  returns them to the vendor. The party on a return records who took the goods and
  limits nothing. A supplier return cannot exceed global on-hand or the live rented
  quantity of that product still here, whoever sent it; a sale cannot exceed global
  on-hand or live purchased quantity for that product less earlier sales. Every check
  repeats inside `Store.UpdateFinance`. The product picker offers everything that may
  leave the store, with an optional tick that narrows it to one supplier's goods.
- Financial detail remains encrypted under spec 17, but ordinary stock arithmetic must
  work after restart without a login. A minimal non-financial disposal projection is
  therefore stored in the public register; supplier/buyer/money remain encrypted.

## Contract

### Inputs

Append to `FinanceData` immediately before `Audit`:

```go
SupplierReturns []SupplierReturn `json:"supplierReturns"`
Sales           []StockSale      `json:"sales"`
```

Normalize both to `[]`. This spec adds audit kinds `supplier_return_created`,
`supplier_return_edited`, `supplier_return_voided`, `sale_created`, `sale_edited`, and
`sale_voided`.

#### Public stock projection

Append to `Register`, before `Finance`:

```go
Disposals []InventoryDisposal `json:"disposals"`
```

```go
type InventoryDisposal struct {
    ID          string               `json:"id"` // "DSP-0001"
    ProductID   string               `json:"productId"`
    Quantity    int                  `json:"quantity"` // >= 1
    Sources     []DisposalAllocation `json:"sources"`  // sum == Quantity
    RecordedAt  time.Time            `json:"recordedAt"`
    InactiveAt  *time.Time            `json:"inactiveAt,omitempty"`
}

type DisposalAllocation struct {
    InwardID string `json:"inwardId"`
    Quantity int    `json:"quantity"` // >= 1
}
```

This projection deliberately contains no kind (`sale`/`supplier return`), party,
amount, purpose, mode, order, remarks, void reason or financial actor. `InactiveAt`
says only that the neutral stock subtraction no longer applies; it cannot reveal whether
that happened through a protected void or public product cascade. It is sufficient for
stock arithmetic and save-time inward guards, and remains human-readable inventory data.
`NextID("DSP")` scans this slice. Schema-1/2 migration normalizes it to `[]`.

Add in `internal/register/arith.go`:

```go
func LiveDisposals(*Register) []InventoryDisposal
func Disposed(*Register, productID string) int
```

For a live product:

```text
OnHand = CameIn - OutWithPeople - Disposed
```

`StockRows`, tiles and every issue guard use this value. `Validate` additionally
reports: a live disposal with unknown/deleted product/inward; source inward for another
product; nonpositive/allocation-sum mismatch; aggregate disposal allocation above a
live inward's quantity; or negative on-hand. An inward quantity correction/deletion is
therefore refused if it would strand an allocation, with:
`Some of this stock has already left the store. Fix that return or sale first.`
While any active disposal allocates an inward, ordinary correction also refuses changing
that inward's `Basis` or `Supplier` with the same neutral sentence, because the public
route cannot decrypt whether the protected allocation is a sale or supplier return.
Increasing quantity, or decreasing it no lower than its allocated total, remains valid.

Spec 15's `DeleteProductCascade` also sets `InactiveAt` on every live disposal for the product in
the same operation. `ProductImpact` gains `DisposalEntries int` and the confirmation
adds `Stock removal entries: <n>`. Protected supplier-return/sale history is not erased
or voided; it later displays `The product was removed from the working register.`
`ProductDeletionImpact.Version` includes every disposal for the product, sorted by ID,
in its deterministic projection. Its save-time stale check runs under the same store
mutex as `UpdateFinance`: if settlement creation wins, deletion must show the refreshed
impact; if cascade wins, settlement creation refuses the deleted product. The encrypted
envelope itself is not part of the public impact hash.

#### Protected settlement types

```go
type SupplierReturn struct {
    ID           string               `json:"id"` // "SRN-0001"
    DisposalID   string               `json:"disposalId"`
    PartyID      string               `json:"partyId"`
    Product      FinanceProductRef    `json:"product"`
    Sources      []DisposalAllocation `json:"sources"`
    ReturnedAt   time.Time            `json:"returnedAt"`
    Reference    string               `json:"reference,omitempty"`
    Remarks      string               `json:"remarks,omitempty"`
    RecordedAt   time.Time            `json:"recordedAt"`
    RecordedByID string               `json:"recordedById"`
    Changes      []FinanceChange      `json:"changes,omitempty"`
    Voided       *FinanceVoid         `json:"voided,omitempty"`
}

type StockSale struct {
    ID           string               `json:"id"` // "SAL-0001"
    DisposalID   string               `json:"disposalId"`
    BuyerPartyID string               `json:"buyerPartyId"`
    Product      FinanceProductRef    `json:"product"`
    Sources      []DisposalAllocation `json:"sources"`
    SoldAt       time.Time            `json:"soldAt"`
    Reference    string               `json:"reference,omitempty"`
    Remarks      string               `json:"remarks,omitempty"`
    RecordedAt   time.Time            `json:"recordedAt"`
    RecordedByID string               `json:"recordedById"`
    Changes      []FinanceChange      `json:"changes,omitempty"`
    Voided       *FinanceVoid         `json:"voided,omitempty"`
}
```

Quantity is the sum of `Sources` and is not duplicated in protected JSON. Every
non-voided settlement whose product is live has one active disposal with identical
product, sources, quantity and `RecordedAt`; every active disposal has exactly one such
protected settlement. A non-voided settlement whose product was later cascade-deleted
is the sole exception: its disposal is inactive, its protected row remains historical,
and validation requires the product tombstone time to be no earlier than settlement
recording and the disposal inactive time to equal that product tombstone time. A voided
settlement requires an inactive disposal at the void time. Finance validation enforces
these states and rejects every other orphan/mismatch. Actor snapshots are carried only
inside encrypted audit/change/void records as spec 19.

#### Source availability and pooled-stock rule

Supplier returns allocate oldest eligible inward first (`ReceivedOn`, then
`RecordedAt`, then ID). Eligible means live, same live ProductID, `Basis == rent`, and
the inward's `FoldKey(Supplier)` equals the selected party's current folded name or any
of that party's prior rename/merge source names. Blank supplier never matches. Subtract
all allocations of live supplier returns from each inward. Exact availability is:

```text
supplier available = sum remaining eligible inward quantities
allowed return     = min(OnHand before this disposal, supplier available)
```

Party aliases belonging to two live parties make the source ambiguous and validation
refuses list rename/merge until one party is merged; no inward may be counted twice.

Sales allocate oldest live `Basis == purchase` inward for the product first, regardless
of supplier. Subtract allocations of live sales. Exact availability is:

```text
purchased available = sum remaining purchased inward quantities
allowed sale         = min(OnHand before this disposal, purchased available)
```

This is attribution, not lot tracking for desk issues. When 390 rented and 310 purchased
Chairs are pooled, chairs returned by people replenish the shared on-hand pool; a
supplier return still cannot exceed that supplier's unreturned rented receipts, and a
sale cannot exceed unsold purchased receipts. The program cannot know which physical
chair is which and makes no stronger claim.

Add pure register functions that read no clock/I/O:

```go
func SupplierReturnAvailable(*Register, *FinanceData, partyID, productID string) int
func AllocateSupplierReturn(*Register, *FinanceData, partyID, productID string, quantity int) ([]DisposalAllocation, error)
func PurchasedAvailableToSell(*Register, *FinanceData, productID string) int
func AllocateStockSale(*Register, *FinanceData, productID string, quantity int) ([]DisposalAllocation, error)
func SupplierObligations(*Register, *FinanceData) []SupplierObligation

type SupplierObligation struct {
    PartyID    string // empty until an existing finance party matches
    PartyName  string
    ProductID  string
    ProductName string
    Received   int
    Returned   int
    Remaining  int
}
```

`SupplierObligation` groups actual live rented inwards by resolved party/product and
reports `Received`, `Returned`, `Remaining`; orders and deposits do not affect it. A
historical nonblank inward supplier with no finance-party match still produces a row
under its cleaned stored name with empty `PartyID`. On the protected return form the
authorized user may deliberately add that exact party through the mandatory typeahead;
the save transaction creates it, then re-resolves and allocates the matching sources.
An unmatched name is never auto-created by a GET.

#### Recording a supplier return

`GET, POST /finance/supplier-returns/new` heading `Return rented goods` contains
mandatory protected party typeahead labelled `Supplier`, select-only live `Product`,
read-only `Available to return`, `How many`, `Date and time returned`, optional
`Reference`, `Remarks`, and button `Save supplier return`. The product suggestions show
only products with positive supplier availability after a party is selected. No field
asks for an inward/order/internal ID.

POST resolves and allocates again inside one `UpdateFinance`. Zero/non-whole number
refuses `Type how many were returned to the supplier.` Above allowed refuses
`Only <allowed> <product name> can be sent back.` Successful save
appends protected `SupplierReturn`, public `InventoryDisposal`, and audit atomically;
redirect 303 says `Supplier return saved.`

A deposit received back is not part of this form. Record it separately as MoneyIn with
purpose `Deposit refund`, optionally linked to this supplier return under the settlement
links below. Supplier return never creates an automatic refund or money row.

#### Recording a stock sale

`GET, POST /finance/sales/new` heading `Record a sale` contains mandatory party
typeahead `Buyer or other party`, live product picker limited to positive purchased
availability, `Available to sell`, `How many`, `Date and time sold`, optional
`Reference`, `Remarks`, and `Save sale`. Save-time allocation/atomicity mirror supplier
return. Above allowed refuses `Only <allowed> <product name> can be sold.` Success says
`Sale saved.`

Sale proceeds are one or more independent MoneyIn rows; no amount is present on the sale
form and no automatic money movement occurs. This permits installments or a sale before
final payment.

Amend `MoneyMovement` with:

```go
Settlements []FinanceSettlementRef `json:"settlements,omitempty"`

type FinanceSettlementRef struct {
    Kind string `json:"kind"` // "supplier_return" | "sale"
    ID   string `json:"id"`
}
```

The create/edit money form always contains an optional labelled selector `Related stock
return or sale`, even when there are none (disabled choice `No returns or sales
recorded yet`). Its choices are visible labels `Supplier return SRN-0001 — <quantity>
<product> — <party>` and `Sale SAL-0001 — <quantity> <product> — <buyer>`; the user
never types an ID. Selecting one stores the matching `FinanceSettlementRef`. This
selector is required UI for recording deposit refunds and sale proceeds; selection is
not mandatory for unrelated money.

References must identify live or voided historical settlements and may coexist with an
order/products. The journal displays `Supplier return SRN-0001` or `Sale SAL-0001`; it
never combines their dates/amounts or implies simultaneous events.

Movement correction may change these references. Insert `settlements` / `Related stock
return or sale` immediately after `products` / `Related products` in spec 19's ordered
movement-change fields; From/To uses the same visible choice labels joined by `; `, or
`Blank`.

#### Correction, void, history

Settlement edit routes allow quantity, occurrence time, party, reference and remarks.
Product is immutable and rendered as a read-only labelled value, followed by `Wrong
product? Void this entry, then record it again with the correct product.` It is never a
disabled select or hidden editable field. A POST containing a different `productId`
returns 200 with `The product cannot be changed here. Void this entry and record it
again.` and no write. Every edit reallocates from
scratch inside `UpdateFinance`, updates the paired public disposal, appends ordered
`FinanceChange` rows and audit, and repeats both availability limits excluding its own
current allocation. A party correction on supplier return must have eligible rented
sources. Admin value rename/merge remains the preferred typo correction.

Void requires a reason, marks the settlement `Voided`, sets only the public disposal's
neutral `InactiveAt` to the void time, and appends full actor/reason audit atomically.
Stock is restored. A voided settlement cannot be edited/voided again but stays in protected
history. Product cascade is not a financial void and retains the live historical
settlement plus its original actor/snapshot.

Before voiding either settlement, show `Void this stock movement?` and `Stock will be
put back into the store totals. This entry will stay in Stock returned or sold and in
Financial activity.` For a supplier return append `Any linked money received will not
change.`; for a sale append `Any linked sale proceeds will not change.` Button: `Yes,
void this stock movement`. First step writes nothing; only reason+CSRF+`confirm=yes`
mutates after rechecking allocation/pair state.
The settlement list's first-step control is `Review voiding this stock movement` and
contains no reason field; the reason is requested only after the consequence warning.

Supplier-return change pairs in form order are `quantity`/`How many`,
`returnedAt`/`Date and time returned`, `party`/`Supplier`, `reference`/`Reference`,
`remarks`/`Remarks`; sale pairs are `quantity`/`How many`, `soldAt`/`Date and time sold`,
`party`/`Buyer or other party`, `reference`/`Reference`, `remarks`/`Remarks`. Quantities
render `<n> <product snapshot>`, times use `2 January 2006 · 3:04 pm`, resolved parties
use their displayed value, and blank optionals render `Blank`.

`GET /finance/settlements` lists supplier returns and sales newest physical time first,
including corrected/void/product-deleted status. `GET /finance/obligations` lists actual
supplier obligations. Both are protected for every financial user.

`GET /finance/obligations` heading is `Rented goods still to return`. Intro:
`Based only on goods actually received and supplier returns recorded here.` Columns are
exactly `Supplier`, `Product`, `Received`, `Returned to supplier`, `Still to return`.
Rows with `Remaining == 0` remain visible as completed history and render `All
returned`; unmatched public supplier names remain visible under `Supplier` without an
action until a protected party is deliberately added.

`InventoryDisposal` never contributes a row to the ordinary `Who did what` log. That
public log must not infer or render settlement kind, financial actor or void reason from
the projection. Full settlement history appears only in protected Financial activity.

### Outputs

- On-hand stock subtracts every live physical supplier return and sale immediately,
  even after restart before financial login.
- Supplier obligations are based on actual supplier-attributed rental receipts, never
  order estimates or deposits.
- Physical exits and money rows can be linked but remain independent audited events.

### Side effects

- Create/edit/void updates protected settlement, public disposal, validation and audit
  in one `UpdateFinance` atomic save.
- Finder/availability/history GETs do not write.
- Deposit refund/proceeds affect money totals only; supplier return/sale affect stock
  only.

## Files to create or modify

- `internal/register/model.go`, `ids.go`, `arith.go`, `product.go`, new
  `finance_settlement.go`, validation and tests.
- `internal/store/store.go`, `finance.go` and tests — copy/migration/paired rollback.
- `internal/web/finance_settlements.go`, corrections, templates and tests.
- Existing inward/product correction tests — disposal allocation guards and cascade.

## Required tests

`TestOnHandSubtractsPublicDisposalsWithoutFinanceUnlock` — receive 100 Tents, return 30
to supplier and sell 20 purchased Chairs; reopen without login and ordinary Stock/issue
guards show exactly 70 fewer Tents/20 fewer Chairs with no protected detail.

`TestSupplierReturnLimitedByOnHandAndSupplierRentalReceipts` — Sharma sent 100 rented
Tents and Gupta 40; with only 60 globally on hand, Sharma allowed is 60; after returning
50, Sharma allows 10 while Gupta also allows only the shared 10. No source is doubled.

`TestSupplierReturnAllocatesOldestEligibleInwards` — Sharma's 70 then 30 partial
receipts allocate 70/20 for a return of 90; purchase, blank-supplier and Gupta inwards
are untouched; party rename retains source attribution.

`TestSaleLimitedByOnHandAndPurchasedReceipts` — 100 purchased and 50 rented Chairs with
60 globally on hand permits at most 60 sold; after 40 sold only 60 purchased basis
remains but global on-hand controls the next amount.

`TestPhysicalSettlementRechecksInsideAtomicUpdate` — two concurrent 40-chair returns
with only 50 allowed accept at most one/full safe combination; same for sales; final
OnHand and source allocations are nonnegative and valid after restart.

`TestDepositRefundHasNoStockEffect` — MoneyIn ₹5,000 Deposit refund linked to SRN-0001
changes received/net totals only; creating/voiding supplier return changes stock only.

`TestMoneyFormHasRequiredSettlementSelector` — create/edit forms render `Related stock
return or sale`, labelled return/sale choices without typed IDs, selected refs persist;
empty state is disabled, and leaving it blank remains valid for unrelated money.

`TestSaleProceedsMayArriveInInstallments` — sale 50 Chairs, then MoneyIn ₹4,000 and
₹6,000 linked to SAL-0001 at different times; stock falls once by 50, journal has two
payments and received total ₹10,000.

`TestSettlementCorrectionReallocatesAndAuditsAtomically` — reduce return 50→30, change
supplier/time/remarks; stock restores 20, sources/cap valid, old/new actor snapshots
persist; forced failure rolls back public and protected halves.

`TestSettlementEditShowsImmutableProductGuidance` — return and sale edit pages render
the exact product as read-only text plus the wrong-product void/re-enter instruction;
hand-built product changes receive the exact refusal with no write.

`TestSettlementVoidRestoresStockButKeepsHistory` — void return/sale with reason; paired
disposal becomes inactive, stock restores, protected row/audit/journal link remain.

`TestSettlementVoidWarnsBeforeSecondConfirmation` — first step shows exact stock,
history and linked-money consequences with byte-identical storage; only
reason+CSRF+`confirm=yes` voids after save-time recheck.

`TestSupplierObligationsUsePlainHeadingAndColumns` — exact heading, intro and five
columns; zero rows say `All returned`, unmatched suppliers remain visible, and no order
or money changes these figures.

`TestInwardCorrectionCannotStrandDisposalAllocation` — reduce/delete an inward below a
live allocated amount, or change its basis/supplier, gets exact refusal and no write;
safe quantity increase/decrease succeeds; void settlement then the formerly blocked
correction succeeds.

`TestProductCascadeTombstonesDisposalsButKeepsFinancialHistory` — impact includes stock
removal count; cascade removes product/entries/disposal from working arithmetic while
protected return/sale/audit and product-name snapshot remain after restart.

`TestInventoryDisposalContainsNoFinancialDetail` — raw schema-3 public JSON projection
has only contracted fields and cannot distinguish sale from supplier return; protected
party/kind/reference/remarks are absent as plaintext.

`TestSupplierObligationUsesActualReceiptsNotOrderOrMoney` — order 100 Tents + advance
creates zero obligation; receive 70 rented from Sharma creates 70; return 20 makes 50;
refund/deposit movement changes nothing.

## Acceptance criteria

1. OnHand subtracts public live disposals and every issue/validation path uses it.
2. Supplier return and sale enforce both global on-hand and attributed-basis caps inside
   the store lock; concurrent requests cannot overspend stock.
3. Public disposal and protected settlement are atomically paired and validated, while
   public JSON leaks no settlement kind/party/money.
4. Money and physical events never auto-create each other and can occur in either order.
5. Inward correction/deletion and product cascade preserve allocation integrity/history.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestOnHandSubtracts|TestSupplierReturn|TestSaleLimited|TestPhysicalSettlement|TestDepositRefund|TestSettlement|TestInwardCorrection|TestProductCascade|TestInventoryDisposal|TestSupplierObligation' -race -count=1 -v
go test ./internal/web/ -run 'TestSupplierReturn|TestSupplierObligations|TestSale|TestDepositRefund|TestMoneyFormHasRequiredSettlement|TestSettlement|TestInwardCorrection|TestProductCascade' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'OnHand|AllocateSupplierReturn|AllocateStockSale|UpdateFinance' internal/web/finance_settlements.go
go doc storeregister/internal/register InventoryDisposal # only ID, ProductID, Quantity, Sources, RecordedAt, InactiveAt
rg -n 'net/http' internal/register internal/store # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. A pooled register cannot prove which physical chair belonged to which supplier or
   basis. The two caps prevent returning/selling more than was actually received under
   that attribution, while the authorized person remains responsible for choosing the
   correct supplier/action.
2. If inventory staff leave `Came from` blank or type a name that matches no protected
   party/alias, those rented receipts cannot be returned to that supplier until the
   inward supplier is corrected. Finance suggestions deliberately cannot leak into the
   ordinary inward form.
