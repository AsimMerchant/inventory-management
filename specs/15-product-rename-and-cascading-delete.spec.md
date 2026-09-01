# Spec: Rename and Retire a Product

## Objective

Let the person at the desk correct a product name from the Stock dashboard and let
them remove a product even when entries already refer to it. A product removal is one
atomic cascading tombstone, never physical erasure: the product and every related
inward, issue and return stop affecting stock and disappear from ordinary screens,
while the JSON file and `Who did what` retain the full audit history.

This is the second of three specs released together as `v1.1.1` and depends on spec 14.

## Context

- Stakeholder decisions recorded 1 September 2026: most wrong-product cases should be
  fixed by renaming; deletion is a rare escape hatch for an expected product that
  never arrived or an entirely wrong product; products with history may be removed in
  one action; the action requires a strong impact summary and a reason; nothing is
  physically erased; product rename and deletion are attributed in the activity log.
- This explicitly supersedes `00-index.spec.md` decision 7 (`No product rename in v1`)
  for `v1.1.1` onward and amends spec 10's statement that `/stock` is read-only.
- Specs 03 and 11 already define record tombstones and the invariant that deleted
  inwards, issues and returns count toward nothing. Spec 12 deliberately reads those
  tombstones. This spec gives `Product` the same append-only correction/deletion
  metadata and cascades one tombstone through all records sharing its `ProductID`.
- Product references remain IDs. A rename therefore changes the displayed name of the
  same stock pool; it never moves quantities between products.

Amendments to prior contracts are explicit: spec 01's `Product` gains two optional
audit fields and its current schema constant becomes 2; spec 02's reader accepts and
migrates schema 1 as detailed below; spec 03's arithmetic additionally treats a
deleted product and its records as absent; spec 06's lookup/duplicate rules range over
live products, while a new log-only mode may see tombstones; spec 10's Stock GET gains
product-management links; spec 12 derives product correction/deletion rows. Spec 11's
individual entry delete guards remain unchanged because this cascade tombstones every
related record in one validated operation.

## Contract

### Inputs

#### File format

Keep every existing `Product` field and append in `internal/register/model.go`:

```go
Changes []Change  `json:"changes,omitempty"`
Deleted *Deletion `json:"deleted,omitempty"`
```

Set `register.SchemaVersion = 2`. `store.Open` accepts schema 1 and 2: after decoding a
schema-1 file, initialize the absent lifecycle fields to their zero values, set the
in-memory `SchemaVersion` to 2, and run the existing `normalise` needed by
open. Opening performs no save and does not change either file's bytes; the next
successful ordinary update writes schema 2 through the existing atomic path. Fresh
registers start at schema 2. Any version other than 1 or 2 retains the existing
different-version refusal.

This bump stops an older executable ignoring `Product.Deleted` and resurrecting a
retired product on its next save: `v1.0.1` only understands schema 1, so it never
parses a schema-2 file and cannot discard the new fields.

It is **not** full downgrade protection, and the difference matters in the field.
`v1.0.1`'s `store.Open` treats a version mismatch exactly like a damaged file: it
copies the main file aside and falls back to `.bak`. Directly after the first
`v1.1.1` save that `.bak` is the schema-1 pre-upgrade file, so an old executable
started at that moment opens, shows the ordinary "the register was damaged, this is
the last good copy" banner, and silently presents pre-upgrade data. Its own next save
writes schema 1 over the main file. Nothing is lost — the schema-2 file survives as
`store-register.json.corrupt-<timestamp>` — but the banner names the wrong cause and
the right remedy ("you are running the old program") appears nowhere.

This cannot be fixed in `v1.1.1`, because the defect is in the already-released
`v1.0.1` reader. The protection is procedural: the old `.exe` must be deleted from
the laptop and the pen drive before the new one is used. Verified by the `v1.1.1`
release gate against real binaries; recorded in `HANDOFF.md`.

Saving an unchanged live product omits both lifecycle fields. `Product.ID`,
`CreatedAt` and `CreatedBy` never change.

Add in `internal/register/product.go`:

```go
type ProductImpact struct {
    ProductID       string
    ProductName     string
    InwardEntries   int
    IssueEntries    int
    ReturnEntries   int
    CurrentlyOut    int
    Version         string
}

func ProductByID(r *Register, productID string) (Product, bool)
func ProductDeletionImpact(r *Register, productID string) (ProductImpact, bool)
func RenameProduct(r *Register, productID, newName, by string, at time.Time) error
func DeleteProductCascade(r *Register, productID, by string, at time.Time, reason string) error
```

`ProductByID` returns only a live product. `ProductDeletionImpact` returns false for an
unknown/already-deleted product. Its three entry counts count related **live** records;
already-tombstoned related records remain audit history but are not presented as being
deleted a second time. `CurrentlyOut` is `register.OutWithPeople(r, productID)` before
the cascade. `Version` is lowercase hex SHA-256 over a deterministic JSON projection of
the product and every related inward, issue and return, including their IDs, editable
values, allocations, changes and tombstones. Slice order cannot change the version:
sort each record set by ID before hashing. The version is a stale-confirmation guard,
not authentication.

`RenameProduct` cleans the new name with `CleanName`, changes only `Product.Name`, and
appends exactly:

```go
Change{
    At: now, By: by, Field: "productName", Label: "Product name",
    From: oldName, To: newName,
}
```

when the cleaned names differ byte-for-byte. Reposting the identical cleaned name is a
successful no-op and appends nothing. The caller enforces the duplicate rules below.

`DeleteProductCascade` requires a live product and a non-empty cleaned reason. It
creates one value:

```go
Deletion{At: now, By: by, Reason: cleanedReason}
```

and assigns it to the product and every **live** inward, issue and return whose
`ProductID` matches. Existing tombstones on related records are preserved byte-for-byte.
It never removes a slice element, clears a correction/allocation, changes a quantity,
or tombstones another product. After applying the cascade, call `Validate`; any problem
aborts the whole update.

All arithmetic/product-list APIs must ignore a deleted product and its records. A
single helper deciding whether a product is live is used by product lookup, picker,
stock rows and product counts; do not scatter HTTP-layer filtering. The activity-log
builder remains the exception and reads deleted products and records.

`internal/store.deepCopy` must independently copy `Product.Changes` and every nested
slice already present on related records. A callback/save failure restores the exact
pre-update register in memory and on disk.

#### Dashboard and product-management routes

Add routes:

```text
GET  /product/{id}/edit
POST /product/{id}/edit
POST /product/{id}/delete
```

Each live product row on `/stock` gains a link `Fix product` to
`/product/<productID>/edit`. The existing `Issue`/`None left` action remains. `/stock`
therefore ceases to be strictly read-only but still performs no mutation on GET.

Unknown product IDs return the existing shell at 404. An already-deleted product
returns 200 with `This product was deleted. Its history is still in Who did what.` and
no edit/delete form.

`GET /product/{id}/edit` uses chrome title and heading `Fix a product`. It shows:

- `Product name`, prefilled with the current name;
- primary button `Save product name`;
- ghost `Cancel` linking to `/stock`;
- a separate bad-coloured disclosure control `Delete this product`.

The deletion disclosure renders the fresh `ProductDeletionImpact`:

```text
Delete Chairs?
This will remove the product and all of its entries from the working register.
Received entries: 3
Issue entries: 4
Return entries: 1
Currently out: 275 chairs
The history will stay in Who did what.
Why are you deleting this product?
One line is enough — "wrong product", "goods never arrived".
```

It posts hidden field `impactVersion`, a text field `reason`, and the
confirmation button `Yes, delete Chairs and its entries`. The reason is required. The
impact block is rendered even when all four figures are zero. The clerk never types or
sees the product ID.

#### `POST /product/{id}/edit`

Field `name`; optional `confirm=yes` only for the near-duplicate confirmation.
Validation order:

1. unknown/deleted product → 404/read-only response above;
2. cleaned name empty → 200, `Type the product's name.`;
3. another live product has the same `FoldKey` → 200,
   `<Existing name> is already on the list. Pick a different name.`;
4. another live product shares the spec 06 four-character folded prefix and
   `confirm != yes` → 200 confirmation:
   `<Existing name> is already on the list. Rename this product to <New name> anyway?`,
   with `No, keep <Old name>` and `Yes, rename it`;
5. otherwise, inside one `store.Update`, re-resolve the live product and re-run rules
   2–4 against current data, call `RenameProduct`, and save.

The product being renamed is excluded from its own duplicate/near-duplicate check.
Success redirects 303 to `/stock?renamed=<productID>`, which shows
`Renamed Chairs to Folding chairs.`. Every live and historical normal display that
resolves this `ProductID` now says `Folding chairs`, including old inward, issue,
return and correction entry names. Old text already stored inside a `Change.From` or
free-text remark remains untouched.

#### `POST /product/{id}/delete`

Fields `impactVersion`, `reason`. Validation order:

1. unknown product → 404; already deleted → read-only 200 response;
2. cleaned reason empty → 200 with a fresh impact block and
   `Write why this product is being deleted.`;
3. missing or mismatched version → 200 with a fresh impact block and
   `This product changed. Check the numbers and confirm again.`;
4. inside one `store.Update`, recompute and compare `impactVersion` again, resolve the
   current on-duty actor, call `DeleteProductCascade`, run `Validate`, and save.

No partial deletion is allowed. A stale version or any function/save error restores
the exact pre-request register in memory and on disk. Success redirects 303 to
`/stock?productDeleted=<productID>`, whose banner uses the deleted record retained in
the raw slice: `Deleted Chairs and all of its entries. The history is still in Who did
what.`

After success, the product and related records are absent from `/stock`, `/out`,
`/inwards`, `/suppliers`, inward/issue/return pickers and all normal correction links.
Their stock effect is zero. Unrelated products and records are byte-for-byte unchanged.

Only live products participate in spec 06 duplicate/near-duplicate guards, so a later
new product may reuse the exact name of a deleted product and gets a new `PRD-` ID.

#### Activity-log audit

Extend spec 12 without adding a stored log table:

- A product rename produces one `LogCorrected` row at `Change.At`, actor `Change.By`,
  record ID/product ID of the product, main line
  `Fixed this product: Folding chairs`, and note
  `Changed the product name from Chairs to Folding chairs`.
- The original product-added row resolves the current name, so it reads
  `Folding chairs added to the product list.` after rename.
- A cascade produces the ordinary original and deletion rows for the product and every
  related record. The product deletion row reads `Deleted this product: Chairs` with
  note `Deleted — Goods never arrived.`. Related record deletion rows retain their
  ordinary `entryName` and the same reason. Original rows are struck and say
  `This entry was deleted later.` exactly as spec 12.
- Product corrections/deletions sort with the existing deterministic rules. The
  cascade rows share one instant and remain deterministic by kind, record ID and
  change index.

Amend `/api/products` with `mode=log`: it includes live and deleted products, never
computes on-hand for deleted ones, and labels a deleted result `<Name> — deleted`.
The spec 14 log picker uses `mode=log`; entry pickers continue using `all`/`instock`
and exclude deleted products. This lets the audit history be selected without putting
a retired product back into a working picker.

### Outputs

- Renaming preserves the product ID and every stock number while updating all
  product-name displays and adding an attributed audit line.
- Cascading deletion makes the product and all related stock movements count toward
  nothing and disappear from working screens in one save.
- The complete original records, corrections, allocations and tombstones remain
  human-readable in `store-register.json` and visible through the activity log.

### Side effects

- Rename and cascade each perform exactly one atomic `store.Update` and one save.
- GET, preview, duplicate/refusal and stale-confirmation paths write nothing.
- The `.bak` sequence remains unchanged. Schema-1 input migrates only in memory on
  open; the first successful update atomically writes schema 2.
- No physical deletion, file replacement outside `store.Update`, or stock arithmetic
  in `internal/web` is permitted.

## Files to create or modify

- `internal/register/model.go`, new `internal/register/product.go`, and tests.
- `internal/register/arith.go` and `log.go` only for centralized live-product filtering
  and derived audit entries.
- `internal/store/store.go` and tests for the new nested product slice rollback.
- `internal/web/products.go`, `views.go`, `log.go`, `saved.go`, `server.go` and tests.
- `internal/web/templates/stock.html`, new `templates/product-edit.html`, and
  `templates/log.html` only as required by the contract.
- `specs/00-index.spec.md`.

## Required tests

`TestSchemaOneMigratesInMemoryWithoutWritingOnOpen` — load the pre-v1.1.1 T3 JSON;
all products are live with nil changes/deletion, totals match, in-memory schema is 2,
and main/backup bytes and modification times are unchanged by open.

`TestFirstSaveAfterMigrationWritesSchemaTwoAtomically` — after that open, perform one
ordinary successful update; main JSON says schema 2, `.bak` is the exact schema-1
pre-update file, and reopen preserves all values.

`TestUnknownSchemaStillRefused` — schema 0 and 3 retain spec 02's safe refusal and no
file is modified.

`TestRenameProductKeepsIdentityAndStock` — rename PRD-0001 Chairs to Folding chairs;
ID, created provenance and every stock number are unchanged; exactly the specified
change is appended.

`TestProductDeletionImpactAtT3` — for Chairs assert the exact live inward/issue/return
counts `3`, `4`, `1`, current outstanding quantity `275`, and a non-empty deterministic
version; reversing all relevant slices produces the same impact/version.

`TestDeleteProductCascadeTombstonesEveryRelatedRecord` — delete Chairs at T3 with
reason `Goods never arrived.`; the product and every Chairs inward/issue/return share
the same actor/time/reason, all remain in their slices, and no unrelated tombstone or
field changes.

`TestCascadePreservesEarlierTombstonesAndCorrections` — a previously deleted Chairs
issue retains its earlier deletion; all correction slices, return allocations and raw
values remain byte-for-byte present after the cascade.

`TestCascadeMakesNoStockImpossible` — after deleting Chairs, `Validate` passes,
`CameIn`, `OutWithPeople`, `Returned` and `OnHand` for PRD-0001 are zero, and every
other product's arithmetic equals the pre-delete register.

`TestStoreRollsBackProductLifecycleSlices` — mutate `Product.Name`, `Changes`,
`Deleted` and related tombstones inside an update that returns an error and inside one
whose save fails; memory and disk equal the pre-update register.

`TestProductEditScreenFromDashboard` — Chairs row contains `Fix product` linked to
`/product/PRD-0001/edit`; GET renders all exact rename/delete labels and the fresh
impact summary, with no visible PRD ID.

`TestRenameChairsToFoldingChairs` — POST succeeds 303, banner is exact, all stock,
inward, out, return/correction and log displays say Folding chairs, while the rename
audit note retains old and new names.

`TestRenameProductRefusesBlankAndDuplicate` — blank and ` round   tables ` produce the
exact 200 refusals, no write and retained typed value.

`TestRenameProductNearDuplicateNeedsConfirmation` — rename Water drums (20L) to
`Round stools` while Round tables is live; first POST renders the exact confirmation,
`confirm=yes` succeeds, and no bypass occurs between read and save.

`TestRenameProductRechecksAtSaveTime` — another live product becomes `Folding chairs`
between render and save; the rename is refused and neither product changes.

`TestDeleteProductShowsStrongImpactSummary` — a synthetic Cable reels product with two
inwards, three issues, one return and 15 currently out renders exact count labels,
`Currently out: 15 cable reels`, the audit-history sentence, required reason field,
hidden non-empty version, and `Yes, delete Cable reels and its entries`.

`TestDeleteProductRequiresReason` — blank reason returns 200 with the exact refusal,
same fresh summary, and byte-identical memory/disk.

`TestDeleteProductRefusesStaleImpact` — render the Cable reels summary, add one issue,
then post the old version; 200 exact stale sentence, updated counts/version, no
tombstones and no save from the refused request.

`TestDeleteProductAndHistoryAtomically` — confirm the fresh version; 303 and exact
banner, every related record tombstoned in one disk version, product absent from all
working screens/pickers, all related stock effects zero, and `Validate` passes.

`TestDeleteProductSaveFailureRollsEverythingBack` — force the save failure after the
cascade callback; product and every related record remain live in memory and disk.

`TestDeletedNameMayBeCreatedAgain` — after deleting Chairs, `POST /product/new` may
create a new live Chairs with the next ID; old entries remain tied only to deleted
PRD-0001 and the new pool starts at zero.

`TestProductLifecycleLogIsComplete` — rename then delete Cable reels with history;
assert the exact product correction/deletion lines, actor/time/reason, original struck
rows and one deterministic deletion row per related live record.

`TestLogPickerCanSelectDeletedProduct` — `mode=log&q=cable` returns
`Cable reels — deleted`; selecting it on `/log?day=all` shows its full audit history,
while `mode=all` and `mode=instock` do not return it.

`TestDeletedProductCannotBeManagedAgain` — edit/delete attempts render the exact
read-only sentence, create no second tombstone and write nothing.

## Acceptance criteria

1. All required and pre-existing tests pass with `-race -count=1`.
2. A rename changes no product ID, record product ID, quantity, allocation or stock
   result; every resolved display uses the new name and the log preserves old/new.
3. A confirmed cascade is all-or-nothing and leaves no live related inward, issue or
   return; `Validate` passes and unrelated data is unchanged.
4. `store-register.json` retains the deleted product and every related record with
   their original values plus tombstones; no slice deletion exists in production code.
5. Schema-1 stakeholder files open without an eager write; the first successful save
   writes schema 2 atomically, and older binaries cannot silently discard new fields.
6. Deleted products are absent from all working pickers/screens and available only to
   the audit-log picker through `mode=log`.
7. Duplicate, near-duplicate, stale-impact and reason guards are rechecked inside the
   one `store.Update` callback.
8. `internal/web` performs no stock or impact arithmetic.
9. Standard-library-only Windows amd64 build succeeds with cgo disabled.

## Verification commands

```text
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestRenameProduct|TestProductDeletion|TestDeleteProductCascade|TestCascade' -race -count=1 -v
go test ./internal/store/ -run 'TestSchemaOne|TestFirstSaveAfterMigration|TestUnknownSchema|TestStoreRollsBackProduct' -race -count=1 -v
go test ./internal/web/ -run 'TestProductEdit|TestRename|TestDeleteProduct|TestDeletedProduct|TestProductLifecycleLog|TestLogPickerCanSelectDeleted' -race -count=1 -v
go test ./... -race -count=1
go vet ./...
rg -n 'DeleteProductCascade|ProductDeletionImpact|Validate' internal/web/products.go
rg -n 'append\(.*Products\[:|Products\[.*:\]|append\(.*Inwards\[:|append\(.*Issues\[:|append\(.*Returns\[:' internal --glob '*.go' --glob '!**/*_test.go' # must print nothing
rg -n 'net/http' internal/register internal/store # must print nothing
rg -n 's\.st\.Update' internal/web/products.go   # the rename and cascade each run in one closure
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
```

## Open

1. The approved walkthrough has no product-management screen. All new strings above
   implement recorded stakeholder decisions and require `plain_language_reviewer`
   review; a rewrite must update this contract and its verbatim tests together.
