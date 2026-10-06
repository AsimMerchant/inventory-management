# Spec: Ledger Entry Simplification — Billed, Paid, Received

## Objective

Ship version `v1.3.0` with one protected routine-entry screen that records a supplier
bill, money paid, money received, or any simultaneous combination without a direction
radio or a separate bill form. Preserve every existing financial and inventory record,
including historical order agreements, while keeping bills auditable, encrypted and
printable beside money in the transaction journal.

## Context

- The approved inventory walkthrough in `design/store-register.html` predates financial
  work and supplies the existing shell, accessibility and plain-language conventions;
  it does not show this screen.
- The user's approved 3 September 2026 design is recorded under `Three amount columns,
  and the bill is what creates a debt` in `HANDOFF.md`: the heading is `Record
  something`; one line has parallel `Billed`, `Paid` and `Received` amount boxes; a bill
  and payment at the same instant save together; there is no billed/paid/received radio
  and no separate Record a bill screen.
- That decision also says a supplier bill creates the billed amount. An order's old
  estimated or exact agreed total is an agreement, not a bill. This spec therefore
  does not reinterpret, copy or total `FinanceOrder.AgreedPaise` or `AgreedKind`.
- Specs 17–21 remain authoritative for authentication, encryption, shared parties,
  reusable purposes/payment modes, exact INR, atomic saves, movements, corrections/voids,
  journal filters/printing, supplier returns and sales. This spec supersedes spec 19's
  routine-create direction control and specs 18–21's `Record money` create wording only
  where stated below. It also supersedes their schema-5/raw-public boundary only by the
  exact schema-6 supplier-challan vocabulary below. Purpose remains mandatory and
  reusable by confirmed user decision.
- Supplier running-balance pages, expected-versus-received goods, broad search and any
  GST/tax or document attachment are not part of this release.

Implementation is split into two reviewable slices because the complete change is
larger than one focused day. Slice A is the register/store domain, validation, atomic
rollback and migration compatibility. Slice B is the protected form, journal,
correction/void pages, no-JavaScript path and browser acceptance. Slice B must not merge
unless Slice A's named tests pass under race.

## Contract

### Inputs

#### Public supplier-challan vocabulary and schema 6

Set `register.SchemaVersion = 6`. Append to `register.Register` immediately after
`Parties`:

```go
SupplierChallans []SupplierChallan `json:"supplierChallans,omitempty"`
```

Add `internal/register/supplier_challans.go` with this exact public type:

```go
type SupplierChallan struct {
    ID      string `json:"id"`      // SCH-0001
    PartyID string `json:"partyId"` // public shared Party.ID
    Number  string `json:"number"`
}
```

No creation time, creator, financial-record ID, amount, purpose, mode, audit or usage
flag is public. `Number` is `CleanName`d, nonblank and unique by `FoldKey` among rows
whose parties resolve through `ResolveParty` to the same live party. IDs are unique
`SCH-*`. `SupplierChallanByID` finds a row; `ResolveSupplierChallan` returns its current
row and resolved live party; `MatchSupplierChallans(reg, partyID, query)` returns only
the selected live party's rows, case-insensitive substring, prefix matches first then
alphabetical folded number, capped at eight by the web layer. If legacy party rows are
later merged and two challans become folded duplicates, validation permits the existing
rows and matching returns the lowest SCH ID once; no migration silently combines them.

`GET /api/supplier-challans?partyId=<party-ID>&q=<text>` follows `/api/parties` access:
an on-duty inventory session or authenticated finance session may call it; off-duty
public access follows the shift guard. A missing/unknown party returns 200 `[]`. Each
JSON object contains exactly `id`, `value`, `label`, where value/label are the challan
number. No response contains `partyId` or any protected relationship.

On `GET, POST /inward/new` and inward correction, the existing optional free-text
`Challan no.` remains stored only as `Inward.ChallanNo`. Once a supplier/party is
selected, the screen may show matching public supplier-challan suggestions; choosing
one copies its visible number into the ordinary text field. It never stores an SCH ID,
never creates/renames/deletes a vocabulary row, never auto-selects or prefills a number
from finance, and never refuses a typed number not in the vocabulary. Inward correction
still displays its own stored `ChallanNo`. With JavaScript disabled, a separate optional
`supplierChallanIdChoice` select lists `Supplier · Challan no.` labels; a nonblank
ordinary `challanNo` text field wins, otherwise a selected same-party SCH row is copied
as text during save. Selecting another party's row simply re-renders
`Pick a challan for this supplier, or type the number.` and writes nothing. Issue
creation/correction and spec-16 issue-return search do not call this API and remain
completely separate.

Only an authenticated financial write creates a challan vocabulary row, in the same
atomic transaction as the entry that first uses it. Exact folded number under the same
resolved party reuses its ID. The protected `Reusable lists` page adds `Supplier
challans`, grouped under each live supplier, with rename and delete-if-unused. Rename
changes only `Number` and appends encrypted `supplier_challan_renamed` audit containing
old/new; delete is allowed only when no bill or movement (including voided history)
references the ID and appends encrypted `supplier_challan_deleted`. There is no combine
operation in v1.3; party merge already makes equivalent-party suggestions appear
together. A party with any challan vocabulary row is used and cannot be deleted until
those rows are deleted.

Reuse `POST /finance/lists/{id}/{action}` for `SCH-*` IDs. `rename` posts `value`; a
blank/duplicate says `Type a different challan number for this supplier.` and writes
nothing. `delete` is two-step: the first POST renders `Delete unused challan
“<number>”?` and `It will be removed from future suggestions. This is allowed only
because no financial record uses it.`; the second posts CSRF plus `confirm=yes` and the
button is `Yes, delete this unused value`. A used row says `In use. Rename it instead.`
and refuses with `This challan has been used. Rename it instead.` Exact store operations
are:

```go
func (s *Store) RenameSupplierChallan(vaultKey []byte, actorID, challanID, newNumber string, now time.Time) error
func (s *Store) DeleteSupplierChallan(vaultKey []byte, actorID, challanID string, now time.Time) error
```

Both require an active administrator and use one `UpdateFinance`, with public mutation
and encrypted audit committed or rolled back together. Audit `EntityType` is
`supplier_challan`; rename summary is `Supplier challan corrected` with old/new in
Before/After, and delete summary is `Unused supplier challan removed` with old/blank.

Schema 1–5 files load with an empty non-nil in-memory vocabulary and no open-time write;
schema 6 loads directly. The next successful atomic save writes schema 6. Any version
other than 1–6 retains the different-version refusal. No existing `Inward.ChallanNo` or
`Issue.ChallanNo` is migrated into this vocabulary.

#### Encrypted model and compatibility

Append `Bills` immediately before `Movements` in `FinanceData`:

```go
Bills []FinanceBill `json:"bills,omitempty"`
```

Add these exact types in `internal/register/finance_bill.go`:

```go
type FinanceBill struct {
    ID                 string              `json:"id"` // BIL-0001
    AmountPaise        int64               `json:"amountPaise"`
    OccurredAt         time.Time           `json:"occurredAt"`
    PartyID            string              `json:"partyId"`
    OrderID            string              `json:"orderId,omitempty"`
    Products           []FinanceProductRef `json:"products,omitempty"`
    PurposeID          string              `json:"purposeId"`
    SupplierChallanID  string              `json:"supplierChallanId,omitempty"`
    Reference          string              `json:"reference,omitempty"`
    Remarks            string              `json:"remarks,omitempty"`
    RecordedAt         time.Time           `json:"recordedAt"`
    RecordedByID       string              `json:"recordedById"`
    Changes            []FinanceChange     `json:"changes,omitempty"`
    Voided             *FinanceVoid        `json:"voided,omitempty"`
}

func (b FinanceBill) Live() bool
func BillByID(f *FinanceData, id string) (FinanceBill, bool)
func SortedBills(rows []FinanceBill) []FinanceBill
func AscendingBills(rows []FinanceBill) []FinanceBill
func TotalBills(f *FinanceData, keep func(FinanceBill) bool) (int64, error)
```

`Live` is `Voided == nil`. Bill sorting uses `OccurredAt`, then ID, with the same
descending/ascending rules as movements. `TotalBills` skips voided bills, applies
`keep` when non-nil and uses `AddPaise`; overflow returns `ErrMoneyOverflow`.

Add to `MoneyMovement`, immediately after `Products`:

```go
SupplierChallanID string `json:"supplierChallanId,omitempty"`
```

`SupplierChallanID` may be blank. When nonblank, combined validation requires an
existing public challan whose resolved party equals the record's resolved `PartyID`.
Vault-only validation requires only a syntactically nonblank SCH reference because the
public register is unavailable there. `Reference` and `Remarks` remain cleaned optional
protected text.

`FinanceBill` validation requires: unique nonblank `BIL-*` ID; positive
`AmountPaise`; nonzero `OccurredAt` and `RecordedAt`; a nonblank public `PartyID`; an
existing `RecordedByID`; a required resolvable `FinancePurpose`; valid changes/void
actor; and, when `OrderID` is nonblank, an
existing order. Every product snapshot must have a nonblank product ID and cleaned
nonblank name. When an order is named, `Products` must equal that order's line snapshots
in order. When no order is named, `Products` must be empty. Public party existence is
checked by the combined register/vault validation boundary, as for current movements;
vault-only validation requires only a nonblank party ID.

Every submitted row has one mandatory reusable Purpose. Resolve or create it through
spec 18 inside the atomic write. The bill plus every Paid/Received movement created from
that row stores the same `PurposeID`. `ValidateFinance` continues to refuse blank or
unresolvable movement purpose and applies the same rule to bills. Purpose suggestions,
admin rename/merge/delete-if-unused and encrypted audit remain spec 18; a purpose used
by a bill, including a voided bill, is used. Simultaneous amounts never create separate
purposes and correction of one generated record never silently changes another.

`FinanceData.NextID` supports `BIL`. `PartyIsUsed` additionally treats any bill and any
supplier-challan vocabulary row as use. `FinanceValueIsUsed` treats a bill's PurposeID,
including voided history, as use. A supplier challan is used when any bill or movement,
including voided history, references it. An order line is referenced when a live bill's
order contains it, as well as under spec 18's movement rule; the order edit screen
therefore cannot remove a product from a live billed order.

Do not increment `vaultVersion`; increment outer `register.SchemaVersion` to 6 solely
for the public supplier-challan vocabulary. A missing `bills` member
means an empty list and loads without an open-time write. The first successful finance
write may serialize the encrypted member; after load/migration, refused/read-only work
must leave main, backup and the post-load in-memory state byte-identical. Existing orders and movements are never
rewritten during load or save merely because `Bills` was added. In particular:

- `FinanceOrder.AgreedPaise == 2500000` with `AgreedKind == "estimated"` remains an
  estimate and contributes zero to `TotalBills`;
- `FinanceOrder.AgreedPaise == 2500000` with `AgreedKind == "exact"` remains an exact
  agreement and contributes zero to `TotalBills`;
- no migration creates a `FinanceBill` from either value;
- a new order created by this screen always has `AgreedPaise == nil` and
  `AgreedKind == ""`.

The program/release version is `1.3.0` and the Git tag is `v1.3.0`. Product version and
schema version are different: this release writes outer schema 6 and encrypted vault 1.

#### Canonical routine form

Keep canonical route `GET, POST /finance/movements/new` so bookmarks and protected
navigation continue working. Change its page title/heading and dashboard link to exact
text `Record something`. Do not add `/finance/bills/new` and do not retain a separate
`Record money` or `Record an order` creation link. `GET /finance/orders/new` redirects
303 to `/finance/movements/new`; `POST /finance/orders/new` is 405 and writes nothing.
Existing `GET /finance/orders`, `GET /finance/orders/{id}` and correction/cancellation
routes remain available for historical and v1.3-generated orders.

The dashboard explanation becomes exactly `Bills, money paid or received, and stock
returned or sold are recorded separately.` Its empty latest-entry text is `No entries
recorded yet.` The Orders list note becomes `Product details are recorded on Record
something. These are the orders recorded so far.` with `Record something` linked to
`/finance/movements/new`.

One draft row uses these exact POST names and visible order:

| Visible label | POST name(s) | Rule |
|---|---|---|
| `Date and time` | `occurredAt` | required local `datetime-local`, defaults to now |
| `Supplier` | `partyId`, `partyName`; no-script `partyIdChoice`, `partyNameNew` | required shared public party picker |
| `Purpose` | `purposeId`, `purposeName`; no-script `purposeIdChoice`, `purposeNameNew` | required encrypted reusable purpose picker |
| `Payment mode` | `modeId`, `modeName`; no-script `modeIdChoice`, `modeNameNew` | required when Paid or Received is nonblank; blank is valid for bill-only |
| `Challan no.` | `supplierChallanId`, `supplierChallanNo`; no-script `supplierChallanIdChoice`, `supplierChallanNoNew` | optional public party-scoped supplier-challan picker; typed new number is allowed |
| `What this is for` | section heading only | contains the product lines |
| `Product` | `product-<row>-<line>`, `productName-<row>-<line>` | select existing or use existing deliberate product-confirmation flow |
| `How many` | `qty-<row>-<line>` | required when that product line is used; whole number >= 1 |
| `How these came in` | `basis-<row>-<line>` plus `kindId-...`, `newKind-...` | current Rent/Purchase/Other contract from specs 18/20 |
| `Billed` | `billed` | optional exact positive INR |
| `Paid` | `paid` | optional exact positive INR |
| `Received` | `received` | optional exact positive INR |
| `Reference` | `reference` | optional |
| `Remarks` | `remarks` | optional |

The finance challan control is filtered by the row's selected party. Selecting a
suggestion submits its SCH ID; changing supplier or visible challan text clears that ID.
On POST, a nonblank `supplierChallanNoNew` wins over the no-script select. Otherwise a
valid selected ID is used; otherwise cleaned `supplierChallanNo` reuses an exact folded
match or creates a new row under the selected supplier. Both ID and text blank means no
challan. An ID owned by another resolved party is refused, never silently reassigned.

The visible acquisition choices retain the current strings `On rent — goes back to
supplier`, `Bought — does not come back`, and `Some other way — donated, sponsored,
borrowed`. The buttons are exactly `Add another product`, `Take this product off this
entry`, `Add another amount`, `Take this amount off`, and `Save`.

There is no direction radio, singular `amount`, `agreedTotal`, `Part of an earlier
order?`, `Related products`, or `Related stock return or sale` control on the new-create
page. Those values on existing records remain visible/correctable on their owning
historical screens. Supplier returns, sales and their later money are still separate
actions; entering `Received` never creates, voids or infers a return or sale.

Each `Add another amount` POST is a draft-only action: it writes nothing and appends one
complete row. It copies the preceding row's date/time, party, purpose, payment mode,
supplier challan and reference, starts Billed/Paid/Received blank, and starts with one blank
product line and blank remarks. This button is for a genuinely separate time, payment
mode, supplier challan, reference or product set; it is not needed to record a bill and
payment at the same instant. Removing a row/line also only redraws and preserves all
other submitted non-secret values.

Within each row, all three amount boxes may be independently blank or filled. At least
one must be filled. Parse each nonblank value with the existing integer-paise parser;
commas, signs, zero, exponents, more than two decimals and overflow are invalid. Validate
in this exact order, stopping at the first error and writing nothing:

1. CSRF/session under spec 17;
2. all three amount boxes blank → `Type a billed, paid or received amount.`;
3. invalid Billed/Paid/Received, left to right → `Type each amount in rupees, using up to two decimal places.`;
4. invalid date/time → `Pick the date and time of the entry.`;
5. unresolved supplier → `Say which supplier this is for.`;
6. unresolved/blank purpose → `Say what this entry is for.`;
7. Paid or Received present with unresolved/blank mode → `Say how the money was paid or received.`;
8. a nonblank challan ID/text that cannot resolve or create under the selected supplier
   → `Pick this supplier's challan from the list, or type a new number.`;
9. product and quantity/basis/kind validation, using the current exact picker refusals;
10. checked bill, paid, received, net and existing order totals overflow → `That amount is too large to total safely.`.

A bill-only row must not require or create a payment mode. If a valid mode is submitted
on a bill-only row, preserve it on a refusal but create no mode merely for that row and
store no mode on the bill. A row with both Paid and Received uses the one selected mode
for both movements. The one selected purpose applies to Billed, Paid and Received.
All form values are re-rendered after refusal; password/recovery
secrets remain governed by spec 21 and are not fields on this form.

#### Atomic record creation

Resolve/create the shared party, mandatory purpose, optional payment mode, optional
public supplier challan, product and custom acquisition kind only inside one
`Store.UpdateFinance`. Validate every submitted row before appending any record or audit
event. Assign IDs in form order, with Billed before Paid before Received inside a row.

For one row:

1. If at least one product line is used, create exactly one `FinanceOrder` containing
   those lines, with `OrderedAt == OccurredAt`, status `open`, nil agreement fields and
   the authenticated creator. A used product line always requires its quantity and
   acquisition kind; an entirely blank product line is ignored.
2. If Billed is present, create one `FinanceBill`. It links the new order when one was
   created; otherwise it has no order. Its products are the order-line snapshots in
   order, or empty when no order was created. It stores the row's PurposeID and optional
   SupplierChallanID.
3. If Paid is present, create one `MoneyMovement` with `Direction == MoneyOut`.
4. If Received is present, create one `MoneyMovement` with `Direction == MoneyIn`.

Both movements link the new order and all its line IDs when an order was created,
freeze the same product snapshots, carry the same supplier challan/reference/remarks,
use the selected payment mode and the same mandatory purpose. Recording Billed/Paid/Received
changes no inward, issue, return,
disposal, supplier obligation or on-hand quantity.

Append one audit per created object: existing `order_created` for the order;
`bill_created` / entity `bill` / summary `Bill recorded · <amount> · <party>`;
and existing `movement_created` for each movement. All records/audits share actual save
time and authenticated account identity. If any validation, callback, encryption,
temporary write, fsync, backup rename or final rename fails, no ID/value/product/kind,
challan, order, bill, movement or audit survives in memory, main or backup.

Success redirects 303 to `/finance/journal?saved=<first-created-ID>&n=<event-count>`.
`event-count` counts bills and movements, not the supporting order. Notices are exactly
`Entry saved.` for one event and `<n> entries saved.` for more than one. Examples:

- Billed ₹20,000 and Paid ₹5,000 creates one bill plus one outgoing movement;
- Paid ₹5,000 with Billed blank creates only one outgoing movement (an advance);
- Billed ₹20,000 alone creates only one bill and requires no payment mode;
- Received ₹5,000 alone creates only one incoming movement;
- Billed ₹20,000, Paid ₹5,000 and Received ₹2,000 creates three live events atomically.

#### Corrections, voids, audit and journal

Keep movement edit/void behavior and routes from spec 19. Add
`SupplierChallanID` to movement edit after `Payment mode` and before `Reference`; it uses
the selected party's public challan picker and its change pair is
`supplierChallan` / `Challan no.` with resolved visible numbers. Purpose remains
mandatory. Correcting party requires clearing or selecting a challan belonging to the
new resolved party in the same POST. Direction edit remains available on the existing
movement correction screen; the no-direction rule is only for routine creation.

Add canonical protected routes:

| Method | Route | Result |
|---|---|---|
| GET, POST | `/finance/bills/{id}/edit` | audited bill correction |
| POST | `/finance/bills/{id}/void` | two-step audited bill void |

Bill edit supports amount, date/time, party, purpose, supplier challan, reference and remarks;
the linked order/products are immutable there and are corrected through the existing
order edit route. Change order is amount, occurred time, party, purpose, supplier challan,
reference, remarks, with exact field/label pairs `amount`/`Billed`,
`occurredAt`/`Date and time`, `party`/`Supplier`,
`purpose`/`Purpose`, `supplierChallan`/`Challan no.`, `reference`/`Reference`,
`remarks`/`Remarks`.
Identical cleaned submission performs no save. Corrections append `FinanceChange` and
one `bill_edited` audit; original values/actor remain.

Bill void follows spec 19's two-press mechanics and immutable `FinanceVoid`. First step
writes nothing and says:

`Void this bill?`

`Use this only if this bill entry was a mistake. It will stop counting in billed totals,
but it will stay in the journal and financial activity. If the supplier cancelled or
replaced a real bill, keep this bill and record what actually happened in Remarks.`

The reason label is `Why are you voiding this bill?` and final button is `Yes, void this
bill`. Only CSRF, `confirm=yes`, nonblank reason and a still-live bill mutates. Append
`bill_voided`; a voided bill remains encrypted, journaled and excluded from totals and
cannot be edited/voided again. There is no physical-delete function or route.

Extend `GET /finance`, `/finance/journal` and `/finance/journal/print` to include bills
and movements in one chronological stream. For equal `OccurredAt`, sort by the numeric
creation sequence represented by ID assignment within a save: Billed, Paid, Received;
when events came from unrelated saves, compare the full stable ID strings after event
kind (`BIL` before `MOV`). Screen is newest occurrence first but keeps Billed before
Paid before Received for one timestamp; print is oldest first with that same tie order.
Each bill row's kind is `Billed`; movement kinds are `Paid` and `Received`. Bill rows
show amount, date/time, supplier, purpose, products/order, challan, reference, remarks,
recorded-by identity/time, corrections and void. Movement rows additionally show mode.
Every missing optional value renders `—` in print.

Filters include both bills and movements using their `OccurredAt`; invalid-filter
behavior remains spec 19. The dashboard and journal tiles are exactly `Billed`, `Paid`,
`Received`. `Billed` sums live bills; `Paid` and `Received` are the existing live money
totals. Do not show a balance, amount due, profit, loss, debit or credit in this release.
Existing `Net paid` arithmetic remains available to order/detail internals and existing
tests but is removed from these two summary tile groups.

Journal actions are `Fix this bill` / `Review voiding this bill` for bills and retain
the current transaction actions for movements. The printable table heading changes
`Paid or received` to `Entry` and includes a `Challan no.` column. Existing historical
agreements may be displayed on order detail exactly as before but never as a `Billed`
journal row or total.

Preserve as separate protected screens and routes: returns, sales, stock-return/sale
history, supplier obligations, journal/print, movement and settlement corrections,
orders/history/corrections/cancellation, accounts, reusable lists, password/recovery
and financial activity. This spec does not collapse the ledger into literally one form.

#### No-JavaScript and privacy contract

With JavaScript disabled, the same POST route provides visible labelled selects for
existing party/purpose/mode/supplier-challan/product/kind values plus the current
explicit new-value/product confirmation paths. Draft Add/Remove buttons, all five
required amount cases, save,
correction, two-step void, filters and print work without typing an internal ID.

`FinanceBill`, amounts, purpose/mode, order links, product associations, actor, time,
reference, remarks and audit remain inside `FinanceEnvelope.Ciphertext`. Schema 6
deliberately exposes only each supplier-challan vocabulary row's `id`, `partyId` and
`number`, alongside the existing public parties/acquisition kinds. The public API emits
only `id`, `value`, `label`. Neither raw public row nor API reveals whether a challan is
used, which financial record uses it, or any protected value. Protected security
headers, CSRF, session expiry, no-store behavior and no external requests remain spec
17/21.

### Outputs

- An authorized person can record bill-only, advance-only, bill plus payment at one
  instant, payment plus receipt, or receipt/refund without choosing a direction; every
  row has one reusable purpose shared by its simultaneous amounts.
- Bills are distinct immutable-history financial events; old agreement values remain
  agreements and cannot silently create debt.
- The transaction journal and print view show Billed/Paid/Received chronologically with
  exact filtered totals and supplier challans.
- The inward desk can manually reuse public supplier challan wording without any
  financial record, amount or purpose becoming public or linked to the inward.
- Inventory behavior and physical return/sale decisions remain independent.

### Side effects

- Draft actions, GETs, refusals, journal filters, print and first-step void pages write
  nothing.
- One successful form submission creates all required products/kinds/party/purpose/
  mode/challan, optional order, bill/movements and audits in one atomic encrypted save.
- Correction and void mutate only the selected protected bill or movement through one
  atomic save and append history; no current or historical event is deleted.
- No action in this spec changes stock.

## Files to create or modify

### Slice A — domain and persistence

- `internal/register/finance_bill.go` (new) — bill model, lookup, ordering and totals.
- `internal/register/supplier_challans.go` (new), `model.go`, `validate.go`, `parties.go`
  — schema-6 public vocabulary, matching, validation and party-use guard.
- `internal/register/finance_model.go`, `finance_ledger.go`, `finance_order.go`,
  `finance_validate.go` — encrypted collection, challan field, IDs, references and
  compatibility validation.
- `internal/register/*_test.go` — bill/legacy-agreement/total/validation tests.
- `internal/store/finance.go` and finance/store tests — deep-copy, absent-list load and
  complete rollback of mixed bill/movement saves.

### Slice B — protected UI and release acceptance

- `internal/web/finance_ledger.go`, `finance_journal.go`, `finance_auth.go` — new draft,
  mixed atomic create, routes, corrections/voids, combined journal and redirects.
- `internal/web/inward.go`, `corrections.go`, `server.go`, supplier-challan API/static
  picker and tests — suggestion-only desk integration with no issue-challan coupling.
- `internal/web/templates/finance-money-form.html`, `finance-home.html`,
  `finance-journal.html`, `finance-journal-print.html`, bill edit/void templates and
  `static/app.css` — exact screen/print/no-script contract.
- Focused `internal/web/*_test.go` — server-rendered form, refusals, journal, privacy and
  backward compatibility.
- `HANDOFF.md` and `.agent-handoff/latest.md` — implementation/browser/gate evidence;
  these are implementation handoff updates, not part of this spec-writing change.

## Required tests

`TestHistoricalAgreementNeverBecomesBill` — load Sharma Tent House orders with
₹25,000 estimated and ₹31,000 exact agreements, reopen and save; Bills remains empty,
both agreement fields are byte-for-byte unchanged and Billed total is ₹0.00.

`TestBillTotalsSkipVoidsAndRefuseOverflow` — bills ₹20,000 and ₹6,000, void the latter;
Billed is ₹20,000. Combining max-safe paise with ₹0.01 returns
`That amount is too large to total safely.` without wrap.

`TestMixedEntryCreatesBillPaidReceivedAtomically` — Asha records 500 Chairs from Sharma
Tent House, purpose Rent, challan STH/4390, Billed ₹20,000, Paid ₹5,000 and Received
₹2,000 at 3 September 2026 12:40; exactly one public SCH row, one nil-agreement order,
one bill, one outgoing and one incoming movement plus four ordered audits persist after
restart, all three financial events share purpose/challan IDs, while stock is zero.

`TestFirstBillWriteRollsBackEveryPublicAndEncryptedChange` — the same mixed entry creates
new product Banquet chairs, party Verma Decor, purpose Event hire, custom kind Borrowed,
challan VD/91 and mode Online;
callback refusal, encryption failure and atomic-file failure each leave main, backup
and in-memory register/vault byte-identical with no orphan IDs or suggestions.

`TestBillOnlyNeedsNoPaymentMode` — Sharma's ₹20,000 Billed-only row with purpose Rent
and blank mode saves one bill/no movement; the same row with all amounts blank says
`Type a billed, paid or received amount.` and writes nothing.

`TestRecordSomethingPurposeIsMandatorySharedAndReusable` — newly typed Event hire is
stored once inside the encrypted reusable-purpose list, copied to simultaneous Billed,
Paid and Received records, and immediately suggested to Rohan. Blank purpose refuses
with `Say what this entry is for.` and leaves no bill/movement/orphan suggestion.

`TestAdvanceOnlyAndRefundOnlyUseParallelColumns` — ₹5,000 Paid-only with Remarks
`Advance`/purpose Deposit and ₹2,000 Received-only with Remarks `Refund`/purpose Deposit
refund create MoneyOut/MoneyIn respectively, no bill, selected Cash mode, correct shared
public challan IDs/reference snapshots and no stock effect.

`TestBillAndPaymentAtSameInstantNeedOneRow` — Billed ₹20,000 plus Paid ₹5,000 has no
direction field in submitted HTML/form, creates both records at the identical occurrence
time and renders Billed then Paid in journal/print.

`TestRecordSomethingValidationIsLeftToRightAndAtomic` — rows containing invalid Billed,
invalid Paid, invalid Received, invalid date, no supplier, no purpose, no payment mode,
wrong-party challan and malformed product quantity each show the exact first refusal,
preserve every submitted value and create no party/purpose/mode/challan/kind/product/
order/bill/movement/audit.

`TestRecordSomethingAdditionalAmountIsDraftOnly` — after the first row contains Sharma,
Cash, purpose Rent, STH/4390, ₹20,000 Billed and Chairs, Add another amount writes
nothing; row two copies date/party/purpose/mode/challan/reference, starts
amount/product/remarks blank, accepts
Bank transfer at a later time, and one final POST saves both rows atomically.

`TestRecordSomethingWorksWithoutJavaScript` — with only server-rendered controls, choose
Sharma Events/purpose Event hire/Cash/challan STH/4390/Chairs, type new kind Sponsored,
record Billed ₹14,000 and Paid ₹4,000, add a second amount and record Received ₹1,000;
all records/audits persist and no internal ID is typed by the test.

`TestBillCorrectionAndVoidKeepCompleteHistory` — correct ₹20,000 to ₹21,500, STH/4390
to STH/4390-A, purpose Rent to Damage charge, party, date, reference and remarks; ordered
changes retain every old/new value and actor. First void step is byte-identical;
confirmed reason excludes the bill from totals but retains journal/audit and refuses
another edit/void. Simultaneous movements keep their original purpose/challan until
explicitly corrected themselves.

`TestMovementChallanCorrectionAndPurposeCompatibility` — old movement with Deposit
purpose edits normally; a v1.3 Paid row also requires a purpose and can change to Refund
plus another same-party challan/reference. Changing party while retaining an invalid
challan refuses atomically; both old and new journal rows survive restart.

`TestSupplierChallanVocabularyIsPublicButUnlinked` — finance creates STH/4390 under
Sharma; raw schema-6 JSON contains exactly public `id`, `partyId`, `number`, while API
returns exactly `id`, `value`, `label`. It contains no amount, purpose, mode, finance
record ID, account, timestamp, audit or usage flag.

`TestInwardChallanSuggestionNeverCouplesTheRecords` — on-duty Suresh selects Sharma and
is offered STH/4390, then saves typed STH/4390-A; inward stores only that text, creates no
SCH row and changes no finance record. No suggestion is prefilled; Gupta sees no Sharma
challan; issue challan search remains unchanged.

`TestSupplierChallanRenameDeleteAndPartyMergeRules` — admin renames STH/439O to
STH/4390 with encrypted old/new audit; a referenced/voided-history row cannot be deleted;
an unused row can. Party merge yields one deduplicated suggestion and no public actor or
usage metadata.

`TestJournalCombinesBillsAndMoneyAcrossAllFilters` — events at 31 August 23:59, 1
September 00:00 and 2 September 18:00 plus simultaneous Billed/Paid/Received at 12:40;
screen/print order, day/date/exact-time inclusion, Billed/Paid/Received totals, challan,
actors, corrections and voids match the contract; old agreements produce no row.

`TestRecordSomethingAndBillsStayProtected` — logged-out/public routes expose no finance
heading, bill, amount, purpose, mode, product relationship or actor; authenticated
responses have all spec-21 headers. Raw schema-6 JSON exposes only the contracted public
party/challan vocabulary and none of the distinctive protected values; finance remains
only the encrypted envelope.

`TestVersion130MigratesSchema5To6AndKeepsOldVaultReadable` — open a released
schema-5/vault-1 fixture containing agreements, movements, returns and sales; read-only
disk bytes are identical while in-memory schema is 6 with empty challans, first bill
save writes schema 6/vault 1, all old records remain exact, and restart validates.

`TestSupplierChallanSchema6MigrationAndFutureRefusal` — valid schema 1, 2, 3, 4 and 5
fixtures all load to in-memory schema 6 with a non-nil empty SupplierChallans slice and
no open-time disk write; the next successful save writes schema 6. Schema 0 and 7 follow
the existing different-version main/backup recovery rules and never load as current.

`TestConcurrentRecordSomethingPostsKeepEveryEvent` — 20 concurrent rows each Paid
₹500 to Sharma via Cash create 20 unique movements, no bills, total Paid ₹10,000,
complete audits and a valid/decryptable restart.

## Acceptance criteria

1. `GET /finance/movements/new` visibly contains, in order, `Record something`, `Date
   and time`, `Supplier`, `Purpose`, `Payment mode`, `Challan no.`, `What this is for`,
   `Product`, `How many`, `How these came in`, `Billed`, `Paid`, `Received`, `Reference`,
   `Remarks`, `Add another amount`, `Save`; it contains no direction radio,
   agreed-total, earlier-order or settlement selector.
2. Bill-only, advance-only, bill+payment, paid+received and received-only cases create
   exactly the contracted records and no stock mutation.
3. A failure at any layer leaves no partial public/encrypted record, suggestion, audit,
   main-file or backup-file change.
4. Historical `AgreedPaise`/`AgreedKind` values load, display and save unchanged and
   contribute zero to Billed totals.
5. Bills and movements are protected, audited, correctable/voidable but never physically
   deleted, and journal/print filters include both.
6. JavaScript-disabled creation, correction, void, filtering and printing complete
   without typing any internal identifier.
7. Finance and inward challans never prefill, overwrite, validate or link one another;
   the desk may manually pick a public suggestion or type any other value, while issue
   challans and return search are unchanged.
8. The raw file writes schema 6 and exposes only public party/acquisition-kind and
   party-scoped supplier-challan vocabulary. No amount, purpose, mode, bill/movement
   relationship, account, timestamp or audit leaks; the cgo-disabled Windows amd64
   build remains a single standard-library binary.
9. Playwright/headless Chromium drives a fresh native build through admin login, the
   five amount combinations, mandatory/reused purpose, public supplier-challan
   suggestion at finance and inward screens, a two-row/different-mode submission,
   correction, two-step bill void, all journal filters, print, logout, raw-file privacy
   inspection and process restart in both JavaScript and JavaScript-disabled contexts.
   Console errors, failed app requests and external requests are zero.
10. Independent plain-language review has no blocking finding and independent
   `release_gate` reports `READY` before tag `v1.3.0` is created.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
git diff --check
go test ./internal/register/ -run 'TestHistoricalAgreement|TestBill|TestMixedEntry|TestAdvanceOnly|TestSupplierChallan|TestVersion130' -race -count=1 -v
go test ./internal/store/ -run 'TestFirstBillWrite|TestVersion130' -race -count=1 -v
go test ./internal/web/ -run 'TestBill|TestRecordSomething|TestAdvanceOnly|TestMovementChallan|TestSupplierChallan|TestInwardChallan|TestJournalCombines|TestVersion130' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register-v1.3.0.exe .
file /tmp/register-v1.3.0.exe
rg -n '^const SchemaVersion = 6$' internal/register/model.go
go list -deps ./... | grep -v '^storeregister' | grep -v '^vendor/' | grep -v '^crypto/internal' | grep '\.'
rg -n 'float(32|64)|ParseFloat|FormatFloat' internal --glob '*.go' --glob '!**/*_test.go'
rg -n 'Bills = (nil|append\([^)]*\[:)|delete\(.*Bill|AgreedPaise.*Bill|AgreedKind.*Bill' internal --glob '*.go' --glob '!**/*_test.go'
rg -n 'SupplierChallanNo|json:"supplierChallanNo' internal/register/finance*.go
rg -n 'name="direction|name="agreedTotal|Part of an earlier order|Related stock return or sale' internal/web/templates/finance-money-form.html
rg -n 'https?://' internal/web/templates internal/web/static
rg -n '0\.0\.0\.0|net.Listen\("tcp", ":' main.go internal --glob '*.go' --glob '!**/*_test.go'
```

The schema-constant `rg` must print exactly its one declaration. The dependency command
and every other `rg` must print nothing; `file` must report PE32+ Windows x86-64.
Browser acceptance uses a fresh `mktemp -d`, a native Linux binary and the
already-installed Playwright/headless Chromium; it must not touch a live
`store-register.json`. Record commit, executable path, discovered loopback URL, browser
version, assertion count, console/network result and restart result in `HANDOFF.md`.

## Open

1. A supplier challan and a tax invoice/bill number may be different documents. Only the
   approved `Challan no.` field is included. A separate invoice number, tax/GST fields,
   attachments and supplier-level balance page require later user decisions/specs.
2. Real Windows 11 double-click launch, executable-relative path permissions and Edge
   printing remain the final stakeholder-machine checks. Linux Playwright plus the
   cgo-disabled Windows cross-build cannot prove those operating-system integrations.
