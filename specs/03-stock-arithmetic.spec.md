# Spec: Stock Arithmetic

## Objective

The numbers the desk trusts. On hand, out with people, what each person is still
holding, and what came in from each supplier — computed from the records, never
stored, never remembered by anyone. These are plain functions with no server, no
disk and no clock of their own, so that every figure in this spec can be proved by a
test.

## Context

- Owns `internal/register/arith.go` and `internal/register/arith_test.go`.
- Depends only on `01-data-model.spec.md`. No imports beyond `sort`, `strings`, `time`.
- All test cases are stated against the named timepoints **T0–T4** defined in
  `01-data-model.spec.md`.

## Contract

### Clock

Every function that can depend on the time takes `now time.Time` as an explicit
parameter. **No function in this package may call `time.Now()`.** This is a contract
requirement, not a style preference: "out over 2 days" is untestable otherwise.

### Deleted records

A deleted record is a tombstone that stays in the file (`01-data-model.spec.md`).
**Every function in this package skips any record whose `Deleted` is non-nil.** There
is no exception and no function that "sees everything" — a deleted entry contributes to
no total, no line, no row, no tile and no count. Only the correction screens
(`11-corrections.spec.md`) and the listing views that render the tombstone read it.

Three helpers exist so no loop hand-writes the check, and every loop in `arith.go` goes
through one of them:

```go
func LiveInwards(r *Register) []Inward
func LiveIssues(r *Register) []Issue
func LiveReturns(r *Register) []Return
```

This rule is easy to satisfy in seven places and miss in the eighth, so it carries its
own cross-cutting test (`TestDeletedInwardVanishesFromEveryNumber`) and its own
acceptance criterion.

### Per-product

```go
func CameIn(r *Register, productID string) int
func OutWithPeople(r *Register, productID string) int
func Returned(r *Register, productID string) int
func OnHand(r *Register, productID string) int
```

- `CameIn` = Σ `Inward.Quantity` for that product.
- `Returned` = Σ over returns of that product of `Return.Quantity()`.
- `OutWithPeople` = Σ `Issue.Quantity` − `Returned`.
- `OnHand` = `CameIn` − `OutWithPeople`.

A shortfall never enters any of these. Five chairs short are five chairs
`OutWithPeople`, whatever the disposition says.

An unknown `productID` returns 0 from all four. Results can never be negative if the
validation in this spec is respected; a negative result is a bug and
`StockRows` must not clamp it — see the audit test.

### Stock table

```go
type StockRow struct {
    ProductID string
    Name      string
    Basis     Basis   // Rent if any inward for the product is rent, else Purchase
    CameIn    int
    Out       int
    OnHand    int
}
func StockRows(r *Register) []StockRow   // sorted by Name, A→Z, case-insensitive
```

A product with no inwards at all still gets a row, with three zeros.

### Per-issue and per-person

**A person is a full name and a mobile number together.** Two people called Ravi Kumar
with different mobiles are two people, each with their own pile. The same name with the
same mobile is one person, however it was capitalised or spaced. Where no mobile was
recorded, the person is keyed on the name alone.

```go
func OutstandingOnIssue(r *Register, issueID string) int  // Issue.Quantity − allocations against it

type PersonID struct {
    NameKey   string // FoldKey(name)
    MobileKey string // MobileKey(mobile); "" when none was recorded
}
func PersonOf(name, mobile string) PersonID

type OutstandingLine struct {
    IssueID     string
    ProductID   string
    ProductName string
    Taken       int
    Back        int
    Out         int        // Taken − Back, always > 0 for a returned line
    IssuedAt    time.Time
    IssuedBy    string     // Issue.PersonInchargeName
}
func OutstandingForPerson(r *Register, name, mobile string) []OutstandingLine
```

`OutstandingForPerson` returns only lines with `Out > 0`, oldest `IssuedAt` first,
ties broken by `IssueID` ascending. An issue belongs to the person when
`PersonOf(issue.TakerName, issue.TakerMobile) == PersonOf(name, mobile)`, so
`"ravi menon"` with `9886140023` and `" Ravi  Menon "` with `98861 40023` are the same
person, while `Ravi Kumar / 98861 40023` and `Ravi Kumar / 90011 22334` are not.

```go
type PersonSummary struct {
    ID          PersonID
    Name        string   // most recent spelling used on an issue
    Department  string   // from the most recent issue
    Mobile      string   // from the most recent issue, shown exactly as typed
    TotalOut    int
    Lines       []OutstandingLine
}
func PeopleHolding(r *Register) []PersonSummary  // only people with TotalOut > 0
func FindPeople(r *Register, query string) []PersonSummary
```

`PeopleHolding` groups by `PersonID` and sorts by `Name` A→Z, ties broken by `Mobile`
so two people of the same name have a stable order.

`FindPeople` is **the one person-finder in the program**. One typed string finds a
person by any of three things, so nobody has to know which box to use:

- a case-insensitive substring of the name,
- a substring of the mobile compared through `MobileKey` on both sides, so `98861`
  finds `98861 40023` and `9886140023` finds it too,
- a case-insensitive substring of the department.

An empty query returns everything `PeopleHolding` returns. The returning screen's
`Find the person` field and the issue screen's `Who is taking it` field both call this
and nothing else — one implementation, one behaviour, tested once.

### Tiles

```go
type Tiles struct {
    Products    int  // len(r.Products)
    OutRightNow int  // Σ OutWithPeople over every product
    PeopleHolding int // len(PeopleHolding(r))
    OutOverTwoDays int // count of outstanding issue lines with IssuedAt before now.Add(-48h)
}
func TileCounts(r *Register, now time.Time) Tiles
```

`OutOverTwoDays` counts **lines**, not items, and counts a line if any quantity on it
is still out.

### Suppliers

**There is no debt in this program.** The store's job ends when the stock is back in
the store; getting rented goods back to the people who own them is somebody else's
work, done outside this software. So there is no `Given back`, no `Still owed`, no
settlement and no money. The Suppliers view is a plain record of what came in from
whom.

```go
type SupplierRow struct {
    Supplier      string  // "" means no supplier was recorded on those inwards
    ProductID     string
    ProductName   string
    OnRent        bool
    CameIn        int     // Σ quantity of that supplier's live inwards of that product
    WontComeBack  int     // Σ ShortQuantity where ShortDisposition == WontComeBack, for this product
}
func SupplierRows(r *Register) []SupplierRow
```

No clock, because nothing here ages.

One row per distinct (`Supplier`, `ProductID`) pair appearing in the live inwards, with
`OnRent` true when that pair's inwards are `rent`. A supplier who sent the same product
both on rent and on purchase produces two rows, because the two facts are different
facts.

`WontComeBack` is a **note on the record, not a qualifier on a debt**: of the 890 chairs
Sharma Tent House sent, 5 are broken and will not be coming back to the store. It is
computed per product and shown on every rent row for that product, so a product with two
rent suppliers repeats the note rather than splitting an unattributable number between
them. Non-rent rows always show 0 — nobody chases stock that was bought.

Ordering: rent rows first, sorted by `Supplier` A→Z then `ProductName` A→Z; then
non-rent rows sorted by `ProductName` A→Z.

### Validate — the one invariant checker

```go
type Problem struct {
    ProductID   string
    ProductName string
    Kind        ProblemKind // NegativeOnHand | NegativeOut | OverAllocatedIssue
    IssueID     string      // set only for OverAllocatedIssue
    Have, Want  int         // the two numbers that disagree
}
func Validate(r *Register) []Problem
```

Three invariants, checked over live records only:

1. `OnHand(product) >= 0` for every product.
2. `OutWithPeople(product) >= 0` for every product.
3. For every live issue, Σ allocations against it from live returns `<= Quantity`.

`Validate` returns an empty slice for every register this program can produce through
the three entry flows. It exists for `11-corrections.spec.md`, which decides whether a
correction is allowed by applying it to a copy and asking this function — rather than by
re-deriving each field's minimum by hand in each handler. One checker with a message
layer on top; never two formulas that have to agree.

### Validation — the two refusals

```go
func CheckIssue(r *Register, productID string, qty int, now time.Time) error
func CheckReturn(r *Register, issueIDs []string, qty int) error
```

`CheckIssue` refuses when: the product does not exist; `qty < 1`; `qty > OnHand`.
`CheckReturn` refuses when: any issue ID is unknown; `qty < 1`; `qty` exceeds
Σ `OutstandingOnIssue` over the given issues.

Errors are typed so the handler can render the right sentence:

```go
type QuantityError struct {
    Field   string // "issue" | "return"
    Asked   int
    Allowed int
    ProductName string
}
func (e QuantityError) Error() string
```

Message text lives in the flow specs, not here.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/register/arith.go`
- `/home/asim/Projects/inventory-management/internal/register/arith_test.go`

## Required tests

Table-driven where the shape repeats. Every case names its timepoint.

### On hand

`TestOnHandAtT0` — Chairs 390, Round tables 48, Water drums (20L) 35,
Extension boards 0, Charcoal sacks 12.

`TestOnHandAfterInward` — at T1, Chairs `CameIn` 1200, `OutWithPeople` 310,
`OnHand` **890**. This is the walkthrough's "After saving, on-hand reads 890."

`TestOnHandAfterIssue` — at T2, Chairs on hand 880, out 320.

`TestOnHandAfterPartialReturn` — at T3, Chairs `CameIn` 1200, `Returned` 45,
`OutWithPeople` 275, `OnHand` **925**.

`TestOnHandMixedSequence` — starting from T0 and applying INW-0007, ISS-0008 and
RET-0001 in order, assert Chairs on hand after each step: 890, 880, 925. Then apply a
second return of the remaining 5 chairs from Ravi and assert 930, and Ravi holds no
chairs.

`TestOnHandUnknownProduct` — `OnHand(T0, "PRD-9999") == 0`.

`TestOnHandNeverNegativeAcrossFixture` — for every product at T0, T1, T2 and T3,
`OnHand >= 0` and `OutWithPeople >= 0`.

### Stock rows

`TestStockRowsAtT0` — exactly five rows in this order: Chairs, Charcoal sacks,
Extension boards, Round tables, Water drums (20L). Chairs row is
`{CameIn: 700, Out: 310, OnHand: 390, Basis: Rent}` — Rent even though 310 of the 700
were purchased. Water drums row is `{40, 5, 35, Purchase}`. Extension boards row has
`OnHand == 0`.

`TestStockRowsIncludeProductWithNoStock` — add product `PRD-0006` "Gas cylinders" with
no inwards; its row reads `{0, 0, 0}` and sorts between Extension boards and Round
tables.

### Issue refusals

`TestIssueRefusedOverOnHand` — at T1 (890 chairs on hand), `CheckIssue` for 900 chairs
returns a `QuantityError` with `Asked: 900, Allowed: 890, ProductName: "Chairs"`.

`TestIssueAllowedAtExactlyOnHand` — at T1, 890 chairs is accepted; 891 is refused.

`TestIssueRefusedWhenNoneLeft` — at T0, any quantity of Extension boards is refused
with `Allowed: 0`.

`TestIssueRefusedForZeroAndNegative` — 0 and −5 chairs are both refused at T1.

`TestIssueRefusedForUnknownProduct` — `CheckIssue(T1, "PRD-9999", 1, T1clock)` errors.

### Outstanding per person

`TestRaviOutstandingBeforeReturn` — at T2,
`OutstandingForPerson(reg, "Ravi Menon", "98861 40023")`
returns three lines in this order:
ISS-0003 Chairs 40 taken / 0 back / 40 out, issued 09:40 by Suresh Kumar;
ISS-0005 Round tables 2 taken / 0 back / 2 out, issued 09:40 by Suresh Kumar;
ISS-0008 Chairs 10 taken / 0 back / 10 out, issued 14:18 by Anita Rao.
Chairs out for Ravi total **50**. This is the walkthrough's returning screen.

`TestRaviOutstandingAfterPartialReturn` — at T3, Ravi has two lines: ISS-0005 Round
tables 2 out, and ISS-0008 Chairs 10 taken / 5 back / **5 out**. ISS-0003 is gone from
the list. "50 issued across two entries, 45 returned, 5 remain out."

`TestPersonMatchIsCaseAndSpaceInsensitive` — at T2, all four of
`OutstandingForPerson(reg, "  ravi   menon ", "98861 40023")`,
`(reg, "Ravi Menon", "9886140023")`, `(reg, "RAVI MENON", "98861-40023")` and
`(reg, "Ravi Menon", "98861 40023")` return the same three lines.

`TestTwoPeopleWithTheSameNameAreTwoPeople` — add to T2 an issue of 6 chairs to
`Ravi Kumar` / `Logistics` / `90011 22334` and another of 4 chairs to `Ravi Kumar` /
`Logistics` / `93400 55118`. `PeopleHolding` reports two separate summaries, one holding
6 and one holding 4, never one holding 10.
`OutstandingForPerson(reg, "Ravi Kumar", "90011 22334")` returns one line of 6.

`TestPersonWithNoMobileIsKeyedOnNameAlone` — add to T2 an issue of 3 chairs to
`Meera Pillai` / `Reception` / `` (no mobile). `PeopleHolding` includes her with
`Mobile: ""` and `TotalOut: 3`, and `OutstandingForPerson(reg, "Meera Pillai", "")`
finds the line. She is a different key from a later `Meera Pillai / 99450 71222` — the
two do not merge, which is accepted (see Open).

`TestPeopleHoldingAtT0` — four people: Farida Begum (5 out), Joseph D'Cruz (145 out),
Lakshmi Iyer (160 out), Ravi Menon (42 out), in that order. Ravi's `Department` is
`Catering` and `Mobile` is `98861 40023`.

`TestPeopleHoldingSortsSameNameByMobile` — with the two Ravi Kumars above, they appear
adjacent and in ascending mobile order, and the order is stable across repeated calls.

`TestFindPeopleByMobileFragment` — at T2, `FindPeople(reg, "98861")` returns exactly
one summary, Ravi Menon, holding 52 items across three lines. This is what Imran types
at 6:05 pm.

`TestFindPeopleByMobileIgnoresSpacing` — `FindPeople(reg, "98861 40023")`,
`FindPeople(reg, "9886140023")` and `FindPeople(reg, "8861400")` each return Ravi Menon.

`TestFindPeopleByName` — `FindPeople(reg, "Ravi Menon")` and `FindPeople(reg, "ravi")`
each return Ravi Menon. With both Ravi Kumars present, `FindPeople(reg, "Ravi")` returns
three summaries — Ravi Kumar twice and Ravi Menon once — and the caller shows all three
rather than resolving between them.

`TestFindPeopleByDepartment` — `FindPeople(reg, "cater")` returns Ravi Menon only;
`FindPeople(reg, "Stage")` returns Joseph D'Cruz only.

`TestFindPeopleExcludesFullyReturned` — after RET-0001 and a further return of Ravi's
last 5 chairs and 2 tables, `FindPeople(reg, "98861")` returns no rows.

### Return refusals

`TestReturnRefusedOverOutstanding` — at T2, `CheckReturn(reg, ["ISS-0003","ISS-0008"], 51)`
returns a `QuantityError` with `Asked: 51, Allowed: 50`.

`TestReturnAllowedAtExactlyOutstanding` — 50 is accepted at T2.

`TestPartialReturnThenOverReturnRefused` — at T3, `CheckReturn(reg, ["ISS-0008"], 6)`
is refused with `Allowed: 5`; 5 is accepted.

`TestReturnRefusedForZero` — `CheckReturn(reg, ["ISS-0008"], 0)` errors.

`TestReturnAgainstFullySettledIssueRefused` — at T3,
`CheckReturn(reg, ["ISS-0003"], 1)` is refused with `Allowed: 0`.

`TestOutstandingOnIssueAcrossTwoReturns` — issue 50 chairs on one line, return 20, then
30. `OutstandingOnIssue` reads 50, 30, 0 after each step, and a further return of 1 is
refused.

### Shortfall does not move stock

`TestShortfallStaysOutstanding` — compare T3 against a variant register where
RET-0001 has `ShortDisposition: ExpectedBack` instead of `WontComeBack`. Chairs on
hand is 925 in both, Ravi holds 5 chairs in both, and the Sharma/Chairs `CameIn` is 890
in both. The only difference anywhere is `WontComeBack`: 5 versus 0.

### Tiles

`TestTilesAtT0` — with `now = 2026-09-03T10:00:00+05:30`:
`Products: 5`, `OutRightNow: 352`, `PeopleHolding: 4`, `OutOverTwoDays: 3`.
The three aged lines are ISS-0001, ISS-0002 and ISS-0007, all issued on 1 September.

`TestOutOverTwoDaysBoundary` — with `now = 2026-09-03T09:45:00+05:30`, ISS-0007
(issued 2026-09-01T09:45) is **not** counted — the boundary is strictly "before
now − 48h" — giving `OutOverTwoDays: 2`. One minute later it counts, giving 3.

`TestOutOverTwoDaysIgnoresReturnedLines` — at T3 with `now = T4`, ISS-0003 is fully
returned, and `OutOverTwoDays` is still 3 because that line was never aged.

### Suppliers

`TestSupplierRowsAtT3` — over the T3 register the rows are, in order:

| Supplier | Product | OnRent | CameIn | WontComeBack |
|---|---|---|---|---|
| Gupta Electricals | Extension boards | true | 25 | 0 |
| Sharma Tent House | Chairs | true | **890** | **5** |
| Sharma Tent House | Round tables | true | 60 | 0 |
| *(blank)* | Chairs | false | 310 | 0 |
| *(blank)* | Charcoal sacks | false | 12 | 0 |
| *(blank)* | Water drums (20L) | false | 40 | 0 |

Sharma Tent House sent 890 chairs; 5 of them are broken and will not come back. No row
carries an owed figure, and `SupplierRow` has no field that could hold one.

`TestSupplierRowsAtT0` — before the 500 arrive, the Sharma/Chairs row reads
`CameIn: 390`, and every `WontComeBack` is 0 because no return has been recorded yet.

`TestSameSupplierBothBasesGetsTwoRows` — add an inward of 20 chairs from
`Sharma Tent House` with `basis: purchase`. The result has a rent row of 890 and a
purchase row of 20 for the same supplier and product, never one row of 910.

`TestWontComeBackShowsOnEveryRentRowOfThatProduct` — add to T3 an inward of 100 chairs
on rent from `Gupta Electricals`. Both the Sharma/Chairs row and the Gupta/Chairs row
report `WontComeBack: 5`; the figure is not split between them and is not doubled in any
total.

`TestGuptaElectricalsRow` — a separate small register reproducing the walkthrough's
third supplier line: one inward of 25 Extension boards on rent from Gupta Electricals
(challan `GE/118`), one issue of all 25 to Joseph D'Cruz, one return of 22 with
`ShortQuantity: 3`, `ShortDisposition: WontComeBack`, remark
`3 boards burnt out at the stage panel.` The row reads `CameIn 25, WontComeBack 3`, and
independently `OnHand == 22`, `OutWithPeople == 3`.

`TestPurchasedStockCarriesNoNote` — the Water drums row has `OnRent == false` and
`WontComeBack == 0`, even after a short return of water drums marked `wont_return`.

### Deleted records

`TestDeletedInwardVanishesFromEveryNumber` — over T0 with `INW-0002` (310 purchased
chairs) tombstoned, assert in one test: `CameIn(Chairs) == 390`; the `StockRows` Chairs
row reads `{CameIn: 390, Out: 310, OnHand: 80}`; `TileCounts.Products` is still 5 and
`OutRightNow` is still 352; and `SupplierRows` has no blank-supplier Chairs row at all.
Four functions, one deletion — this is the test that catches the loop somebody forgot to
filter.

`TestDeletedIssueVanishesFromEveryNumber` — over T0 with `ISS-0001` (150 chairs to
Lakshmi Iyer) tombstoned: `OutWithPeople(Chairs) == 160`, `OnHand(Chairs) == 540`,
`PeopleHolding` still returns four people, and Lakshmi Iyer is one of them with
`TotalOut: 10` and a single line for her 10 round tables — she has not disappeared, she
has simply stopped holding chairs. `FindPeople(reg, "Lakshmi")` returns her with that
one line and no chairs line. `TileCounts` reads `OutRightNow: 202`, `PeopleHolding: 4`,
`OutOverTwoDays: 2` — ISS-0001 was one of the three aged lines.

`TestDeletedReturnVanishesFromEveryNumber` — over T3 with `RET-0001` tombstoned:
`Returned(Chairs) == 0`, `OnHand(Chairs) == 880`, Ravi is back to 50 chairs out across
ISS-0003 and ISS-0008, `OutstandingOnIssue("ISS-0003") == 40`, and the Sharma/Chairs
row reports `WontComeBack: 0` — the note dies with the record that carried it.

`TestDeletedRecordsStayInTheFile` — after all three deletions, the register still has 6
inwards, 7 issues and 1 return in its slices, each tombstone readable with its `By` and
`Reason`.

### Validate

`TestValidateIsCleanForEveryFixtureTimepoint` — `Validate` returns an empty slice for
T0, T1, T2 and T3, and for the register after every successful correction in
`11-corrections.spec.md`.

`TestValidateCatchesNegativeOnHand` — hand-build a register with 10 chairs in, an issue
of 10, a return of 10 and a second issue of 10, then tombstone the return.
`Validate` reports exactly one `Problem`: `{ProductName: "Chairs", Kind:
NegativeOnHand, Have: -10, Want: 0}`.

`TestValidateCatchesNegativeOut` — a register whose returns exceed its issues for a
product reports `NegativeOut`.

`TestValidateCatchesOverAllocatedIssue` — an issue of 40 chairs with two live returns
allocating 30 and 20 against it reports one `OverAllocatedIssue` naming that issue, with
`Have: 50, Want: 40`.

Each of the three kinds must be independently reachable: a checker that only ever trips
the first invariant would pass a single-case test and let the other two through.

## Acceptance criteria

1. `go test ./internal/register/ -count=1` passes, with every test named above.
2. `grep -n 'time.Now()' internal/register/*.go` returns nothing outside `_test.go` files, and nothing at all in `arith.go`.
3. `go test ./internal/register/ -cover` reports at least 95% statement coverage for the package.
4. Every function in the Contract above appears in `go doc storeregister/internal/register`.
5. `go vet ./internal/register/` is clean.
6. **No loop in `arith.go` iterates a record slice directly:**
   `grep -nE 'range r\.(Inwards|Issues|Returns)' internal/register/arith.go` returns
   nothing. Every one goes through `LiveInwards`, `LiveIssues` or `LiveReturns`, which
   are the only three places the `Deleted` check is written.
7. `grep -rniE 'givenBack|StillOwed|\bowed\b' internal/register/ internal/web/`
   returns nothing outside test files — the debt columns are gone, not hidden. The
   word boundary matters: a bare `owed` also matches `Allowed`, a `QuantityError`
   field this same spec mandates and two of its required tests assert.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -count=1 -cover -v
go vet ./internal/register/
grep -n 'time.Now()' internal/register/arith.go   # must print nothing
```

## Open

1. **The same person entered once with a mobile and once without** is two rows — the
   morning issue typed as `Ravi Menon` with no number, the afternoon one as
   `Ravi Menon / 98861 40023`. The picker in `08-issue.spec.md` reduces this by filling
   in the known mobile silently, and index principle "people records may be sloppy"
   says the desk must never be stopped to resolve it. Noted, accepted, not guarded.
2. **"Out over 2 days" counts lines, not items.** The mockup's `3` is consistent with
   either reading. Settled as lines (index decision 17).
3. **48 hours versus two calendar days.** A rolling 48 hours, as specified.
