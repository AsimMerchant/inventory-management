# Spec: The Product List and the Picker

## Objective

If Monday's person types "Chairs" and Wednesday's types "chair", the register owns two
products, the stock count is wrong, and there is no fixing it mid-event. So a product
name is never free-typed into a record: it is chosen from a list the software puts in
front of the person, and creating a brand-new product is a separate, deliberate press.
This is the single highest-value invariant in the system.

**This is the one place the software is allowed to be strict**, and the person picker in
`08-issue.spec.md` is deliberately the opposite: it offers matches and never insists.
The asymmetry is the point. A duplicate person is a cosmetic problem a human can see and
sort out; a duplicate product is an arithmetic problem that silently halves the on-hand
figure and is noticed only after the count is already wrong.

## Context

- Owns `internal/web/products.go`, `internal/web/static/picker.js`,
  `internal/web/templates/picker.html` (a partial included by the inward and issue
  forms), and their tests.
- Depends on `01-data-model.spec.md` (`Product`, `CleanName`, `NextID`) and
  `03-stock-arithmetic.spec.md` (`OnHand`).

## Contract

### The rule, in code

Only `POST /product/new` may append to `Register.Products`. Every other handler
resolves a `productId` submitted by the form against the existing list and **fails the
request** if it does not match. No handler anywhere may create a product as a side
effect of saving an inward, an issue or a return.

### `GET /api/products?q=<text>&mode=<all|instock>`

Returns `application/json`:

```json
[{"id":"PRD-0001","name":"Chairs","onHand":390,"label":"Chairs — 390 on hand"}]
```

- Match: case-insensitive substring of `Product.Name` after `CleanName` on the query.
  An empty `q` returns the whole list.
- `mode=instock` (used by the issue screen) drops products with `OnHand == 0`.
  `mode=all` (the inward screen) keeps them. Default `all`.
- Order: names that *start* with the query first, then the rest; alphabetical A→Z
  within each group, case-insensitive. Cap at 8 results.
- `label` is exactly `<Name> — <OnHand> on hand`, with an em dash, matching the
  walkthrough's suggestion rows.

With `q=Cha` against `WalkthroughT0()` this returns, in order,
`Chairs — 390 on hand` and `Charcoal sacks — 12 on hand`.

### The picker

A text input with `class="inp"`, plus a hidden input `productId` that is the only
thing the form submits. Typing queries `/api/products` after each keystroke and
renders a `div.sug` list:

- one row per result showing the `label`, the first row highlighted (`hl`);
- on the **inward** screen only, a final row with `class="new"` reading
  `+ Add "<what they typed>" as a brand-new product`;
- clicking or Entering a row fills the text input with the product name, sets
  `productId`, and hides the list.

If `productId` is empty when the form is submitted, the form is refused with
`banner bad` `Pick the product from the list.` — a typed name that was never picked
never becomes a record.

Without JavaScript the picker degrades to a `<select name="productId">` listing every
product (filtered by `mode`), inside `<noscript>`. The register must be usable if
JavaScript fails.

### `POST /product/new`

Fields: `name`, and `return` (the path to come back to, `/inward/new` or `/stock`).

1. `n := CleanName(name)`. Empty → 200, `banner bad` `Type the new product's name.`
2. If any existing product has `FoldKey(existing.Name) == FoldKey(n)` → 200,
   `banner bad`
   `<Existing name> is already on the list. Pick it.`
   and nothing is written. This is the case-and-spacing guard: `chairs`, `Chairs `
   and `CHAIRS` can never become second products.
3. Otherwise append
   `Product{ID: NextID("PRD"), Name: n, CreatedAt: now, CreatedBy: <on-duty name>}`
   — the shift guard means somebody is always on duty here, so `CreatedBy` is never
   empty on a product — save, and
   303 back to `return` with `?picked=<new id>` so the form it came from opens with the
   product already chosen and a `banner ok` reading `<Name> added to the product list.`

A product's Rent/Purchase basis is **not** set here — basis belongs to each inward
record (`01-data-model.spec.md`).

### Deleting a product

Not built. A product with records against it cannot be removed without rewriting
history; one with no records is harmless.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/products.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/picker.html`
- `/home/asim/Projects/inventory-management/internal/web/static/picker.js`
- `/home/asim/Projects/inventory-management/internal/web/products_test.go`

## Required tests

`TestSuggestChaMatchesWalkthrough` — `GET /api/products?q=Cha` over `WalkthroughT0()`
returns exactly two results in this order, with these labels:
`Chairs — 390 on hand`, `Charcoal sacks — 12 on hand`.

`TestSuggestIsCaseInsensitive` — `q=cha`, `q=CHA` and `q=  Cha ` all return the same
two results.

`TestSuggestMatchesMidWord` — `q=drum` returns `Water drums (20L) — 35 on hand`.

`TestSuggestPrefixMatchesRankFirst` — after adding a product `Folding chairs stand`,
`q=cha` returns `Chairs` and `Charcoal sacks` before `Folding chairs stand`.

`TestSuggestInStockModeHidesEmpties` — `q=&mode=instock` over `WalkthroughT0()` omits
`Extension boards` (0 on hand) and includes the other four; `mode=all` includes all
five.

`TestSuggestOnHandFollowsTheRegister` — at T1, `q=Chairs` returns
`Chairs — 890 on hand`; at T3 it returns `Chairs — 925 on hand`.

`TestSuggestCapsAtEight` — with 20 products whose names all contain `a`, the response
has exactly 8 entries.

`TestCreateProductAppends` — `POST /product/new` with `name=Gas cylinders` over
`WalkthroughT0()` returns 303, and the saved register has a sixth product `PRD-0006`
named `Gas cylinders`, with `CreatedAt` equal to the injected clock and
`CreatedBy == "Suresh Kumar"`, the person on duty.

`TestCreateProductRefusesCaseDuplicate` — table driven over `chairs`, `CHAIRS`,
`Chairs`, `  chairs  `, `Chairs\t`. Every one returns 200 containing
`Chairs is already on the list. Pick it.` and the register
still has exactly five products.

`TestCreateProductTrimsAndCollapses` — `name=  Gas   cylinders ` stores
`Gas cylinders`.

`TestCreateProductRefusesBlank` — `name=   ` returns 200 containing
`Type the new product's name.` and five products.

`TestOnlyProductRouteCreatesProducts` — post a valid inward for 500 chairs, a valid
issue of 10 chairs and a valid return of 45 chairs; after all three the register still
has exactly five products. Then post an inward whose `productId` is
`PRD-9999`: the response is 200 with `banner bad` `Pick the product from the list.`,
the register still has five products, and no inward was added.

`TestFormWithoutProductIdRefused` — `POST /inward/new` with a `productName=Chairs`
field but no `productId` returns 200 containing `Pick the product from the list.` and
writes nothing. A name typed but not picked must never create or match a product.

`TestNoScriptFallbackListsProducts` — `GET /inward/new` body contains a
`<select name="productId">` inside `<noscript>` with an `<option>` for each of the five
products.

`TestAddNewRowOnlyOnInward` — `GET /inward/new` body contains
`as a brand-new product`; `GET /issue/new` does not.

## Acceptance criteria

1. `go test ./internal/web/ -run 'TestSuggest|TestCreateProduct|TestOnlyProduct|TestForm|TestNoScript|TestAddNew' -count=1` passes.
2. `grep -rn 'Products = append' internal/ | grep -v _test | grep -v fixture` matches
   exactly one line, in `internal/web/products.go`.
3. `grep -c 'FoldKey' internal/web/products.go` is at least 1 — the duplicate guard is
   case-insensitive by construction.
4. `wc -l internal/web/static/picker.js` is under 120 lines and the file contains no
   `import`, no `require`, and no URL.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -count=1 -v
grep -rn 'Products = append' internal/ | grep -v _test | grep -v fixture
```

## Open

1. **Near-duplicates that differ by more than case.** The guard stops `chairs` versus
   `Chairs`; it does not stop `Chair` versus `Chairs`, or `Round table` versus
   `Round tables`. Recommend a confirmation step: when the typed name shares a
   case-folded prefix of 4 or more characters with an existing product, show
   `Did you mean <Existing>? Adding "<typed>" makes a second, separate product.` with
   the existing product as the default action. Not in the walkthrough — confirm before
   building.
2. **Renaming a product** (a typo caught on day two). Not shown. Recommend: not built
   in v1; the file can be edited by hand between sessions.
3. **Whether the issue screen may create a product.** The walkthrough shows the
   `+ Add … as a brand-new product` row only on the inward screen; this spec forbids it
   on the issue screen, since issuing something never received is meaningless. Confirm.
