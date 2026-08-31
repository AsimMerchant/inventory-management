# Spec: Stuff Came In

## Objective

500 chairs arrive at the gate. The person on duty presses **Stuff came in**, picks the
product off the list, types how many, says whether it is on rent or bought outright,
and who it came from. The pool for that product grows: Chairs stood at 390 on hand
before, and reads 890 after.

## Context

- Owns `internal/web/inward.go`, `internal/web/templates/inward.html`, tests.
- Depends on `01` (`Inward`, `Basis`), `03` (`OnHand`), `05` (on-duty person),
  `06` (the product picker).
- Route: `GET, POST /inward/new`. Reached from the `+ Stuff came in` button on
  `/stock`.

## Contract

### Shared: the product word in button labels

Used by this spec and by 08 and 09. Template function `productWord(name string) string`:

> If the product name contains no uppercase letter after the first character and no
> digit, lowercase its first character. Otherwise return it unchanged.

`Chairs` → `chairs`; `Round tables` → `round tables`; `Extension boards` →
`extension boards`; `Water drums (20L)` → `Water drums (20L)` unchanged. This
reproduces the walkthrough's `Save — 500 chairs in` without mangling a name that
carries its own capitals.

### `GET /inward/new`

Chrome title `Stuff came in`. Heading `Record what arrived`. Sub-heading is the
current time through `longstamp` — `Thursday, 3 September · 10:42 am`. No tabs.

Fields, in this order and with exactly these labels:

| Label | Control | Default | Required |
|---|---|---|---|
| `Product` | picker from 06, `mode=all`, with the add-new row | empty, or `?picked=` | yes |
| `How many` | `<input type="number" min="1" step="1">` | empty | yes |
| `Date received` | `<input type="date">` | today | yes |
| `Rent or purchase` | two `opt` buttons: `On rent — goes back to supplier` / `Purchased for resale — does not come back` | `On rent` selected | yes |
| `Came from` | text | empty | no — hint `Leave blank if you don't know.` |
| `Challan no.` | text | empty | no |
| `Received by` | read-only, `<On-duty name> (you)` | on-duty name | — |

`How many` and `Date received` share a `row2`; `Came from` and `Challan no.` share a
`row2`. Buttons: primary `Save — <n> <productWord> in` and ghost `Cancel` linking to
`/stock`. The primary label updates live as the quantity and product change; with no
product or quantity chosen yet it reads `Save`.

`Came from` offers suggestions from suppliers already used in the register
(case-insensitive substring, from a `<datalist>` — no new endpoint), but unlike the
product field a new supplier may be typed freely.

### `POST /inward/new`

Form fields: `productId`, `quantity`, `receivedOn`, `basis`, `supplier`, `challanNo`.

Validation, in this order; the first failure re-renders the form at status 200 with
every entered value still in place and a `banner bad`:

| Condition | Banner |
|---|---|
| `productId` empty or unknown | `Pick the product from the list.` |
| `quantity` not a whole number, or < 1 | `Type how many <Product name> came in.` |
| `receivedOn` not `YYYY-MM-DD` | `Type the date like this: 03-09-2026.` |
| `basis` not `rent` or `purchase` | `Choose rent or purchase.` |

On success, `Update` appends:

```
Inward{
  ID: NextID("INW"), ProductID: productId, Quantity: n,
  ReceivedOn: receivedOn, Basis: basis,
  Supplier: CleanName(supplier), ChallanNo: strings.TrimSpace(challanNo),
  ReceivedBy: onDuty.Name, RecordedAt: now, RecordedBy: onDuty.Name,
}
```

then 303 to `/stock?saved=INW-000n`, where `/stock` shows a `banner ok`:
`Added <n> <Product name>. <Product name>: <OnHand> on hand.` —
for the walkthrough's entry: `Added 500 chairs. Chairs: 890 on hand.`

There is no maximum quantity. Stuff arriving is never refused.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/inward.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/inward.html`
- `/home/asim/Projects/inventory-management/internal/web/inward_test.go`

## Required tests

`TestInwardFormRendersWalkthroughLabels` — `GET /inward/new` over `WalkthroughT0()`
with the clock at `2026-09-03T10:42:00+05:30` contains: `Record what arrived`,
`Thursday, 3 September · 10:42 am`, `Product`, `How many`, `Date received`,
`Rent or purchase`, `On rent — goes back to supplier`,
`Purchased for resale — does not come back`, `Came from`,
`Leave blank if you don't know.`, `Challan no.`, `Received by`,
`Suresh Kumar <span class="sm">(you)</span>` and `Cancel`.

`TestInwardFormDefaultsDateToToday` — the `Date received` input's `value` is
`2026-09-03` at that clock.

`TestInward500ChairsRaisesOnHandTo890` — `POST /inward/new` with
`productId=PRD-0001&quantity=500&receivedOn=2026-09-03&basis=rent&supplier=Sharma Tent House&challanNo=STH/4471`
over `WalkthroughT0()`:
- returns 303 to `/stock?saved=INW-0007`;
- the saved register has 7 inwards, the new one exactly as listed for T1 in
  `01-data-model.spec.md` including `ReceivedBy: "Suresh Kumar"`;
- `register.OnHand(reg, "PRD-0001") == 890`;
- following the redirect, `/stock` contains
  `Added 500 chairs. Chairs: 890 on hand.` and a Chairs row reading
  `1200`, `310`, `890`.

`TestInwardWithBlankSupplierAndChallan` — the same post with `supplier=` and
`challanNo=` succeeds; the stored record has both fields `""` and the file on disk
contains `"supplier": ""`.

`TestInwardRefusesZeroAndNegative` — `quantity=0`, `quantity=-5` and `quantity=abc`
each return 200 containing `Type how many chairs came in.`, and the
register still has 6 inwards.

`TestInwardRefusesFractional` — `quantity=12.5` is refused with the same sentence.

`TestInwardRefusesUnknownProduct` — `productId=PRD-9999` returns 200 containing
`Pick the product from the list.`; six inwards remain.

`TestInwardRefusesMissingBasis` — `basis=` returns 200 containing
`Choose rent or purchase.`

`TestInwardRefusesBadDate` — `receivedOn=03/09/2026` returns 200 containing
`Type the date like this: 03-09-2026.`

`TestInwardKeepsTypedValuesOnRefusal` — a post with a valid supplier
`Sharma Tent House`, challan `STH/4471` and `quantity=0` re-renders with
`Sharma Tent House` and `STH/4471` still in their inputs, so nothing has to be typed
twice.

`TestInwardOfPurchasedStock` — posting 40 `Water drums (20L)` with `basis=purchase`
and a blank supplier stores `Basis: "purchase"`, and `SupplierRows` puts the product on
a non-rent row.

`TestInwardButtonLabel` — `productWord` table: `Chairs`→`chairs`,
`Round tables`→`round tables`, `Extension boards`→`extension boards`,
`Water drums (20L)`→`Water drums (20L)`, `Charcoal sacks`→`charcoal sacks`. And a
rendered form with 500 chairs pre-picked contains `Save — 500 chairs in`.

`TestInwardIsAtomicOnDisk` — after a successful post, `store-register.json.bak` parses
and holds 6 inwards while `store-register.json` holds 7.

## Acceptance criteria

1. `go test ./internal/web/ -run TestInward -count=1` passes with all thirteen tests.
2. `curl`-equivalent in test: the success path returns exactly `303` and
   `Location: /stock?saved=INW-0007`.
3. Every refusal path returns `200`, never `400` or `500`, and no refusal body contains
   the words `invalid`, `error`, `nil` or `panic`:
   `go test ./internal/web/ -run TestInwardRefuses -v` plus a grep assertion inside the
   test.
4. `grep -n 'Products = append' internal/web/inward.go` returns nothing.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestInward' -count=1 -v
go vet ./internal/web/
```

## Open

1. **Pluralisation.** `Save — 1 chairs in` is wrong but the walkthrough never shows a
   quantity of 1. Recommend: no pluralisation logic; product names are stored plural
   and the number carries the sense. Confirm, or supply a singular field per product.
2. **`productWord` casing rule.** Derived from three button labels in the walkthrough.
   Confirm the rule, or say the product name should always appear exactly as stored.
3. **Is `Received by` ever someone other than the person on duty?** The walkthrough
   shows it read-only as `Suresh Kumar (you)`. Recommend keeping it read-only.
4. **Supplier free text.** Products are picked, suppliers are typed with suggestions.
   That is what the mockup shows, but it allows `Sharma Tent House` and
   `Sharma tent house` to become two supplier rows. Recommend applying the same
   case-fold guard as products: a typed supplier that case-matches an existing one is
   silently stored using the existing spelling. Confirm.
