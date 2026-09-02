# Spec: Data Model and File Format

## Objective

Define the shape of everything the store desk remembers: the products it stocks, the
people who can be on duty, and the three kinds of record — stuff came in, someone
took it, it came back. One plain JSON file next to the binary holds all of it, in a
form a person can open in Notepad and read. This spec also defines the **walkthrough
fixture**, the single register that every other spec's test cases are written
against.

Nothing in this spec computes anything. On-hand, outstanding and the supplier rows
all live in
`03-stock-arithmetic.spec.md`.

## Context

- Module path `storeregister`, Go 1.27, `go.mod` at the repository root.
- This spec owns `internal/register/model.go`, `internal/register/ids.go` and
  `internal/register/fixture.go`.
- Standard library only. `encoding/json` and `time`.
- Stock is pooled at product level. An inward record is **not** a lot: issues and
  returns never point at an inward record.

## Contract

### Package and root type

```go
package register

type Register struct {
    SchemaVersion int       `json:"schemaVersion"`
    OnDutyStaffID string    `json:"onDutyStaffId"`  // "" when no shift started
    ShiftStartedAt *time.Time `json:"shiftStartedAt,omitempty"`
    Products      []Product `json:"products"`
    Staff         []Staff   `json:"staff"`
    Inwards       []Inward  `json:"inwards"`
    Issues        []Issue   `json:"issues"`
    Returns       []Return  `json:"returns"`
}
```

`SchemaVersion` is `1`. A file with any other value is refused at load
(`02-persistence.spec.md`).

Slices are never `nil` in a loaded `Register`; an empty register has empty slices so
the file shows `[]` rather than `null`.

### Entities

```go
type Product struct {
    ID        string    `json:"id"`        // "PRD-0001"
    Name      string    `json:"name"`      // "Water drums (20L)"
    CreatedAt time.Time `json:"createdAt"`
    CreatedBy string    `json:"createdBy"` // on-duty staff name when it was added
}

type Staff struct {          // a person who can be on duty
    ID        string    `json:"id"`        // "STF-0001"
    Name      string    `json:"name"`      // "Suresh Kumar"
    Mobile    string    `json:"mobile"`    // "98450 22117"
    CreatedAt time.Time `json:"createdAt"`
    CreatedBy string    `json:"createdBy"` // on-duty staff name when they were added
}

// Every record that can be corrected carries these two. See 11-corrections.spec.md.
type Change struct {
    At    time.Time `json:"at"`
    By    string    `json:"by"`    // on-duty staff name at the time of the correction
    Field string    `json:"field"` // "quantity" | "supplier" | "takerName" | ...
    Label string    `json:"label"` // the on-screen label, e.g. "How many"
    From  string    `json:"from"`  // rendered as the user saw it: "500"
    To    string    `json:"to"`    // "50"
}

type Deletion struct {
    At     time.Time `json:"at"`
    By     string    `json:"by"`
    Reason string    `json:"reason"` // required, plain words
}

type Inward struct {
    ID          string    `json:"id"`          // "INW-0001"
    ProductID   string    `json:"productId"`
    Quantity    int       `json:"quantity"`    // >= 1
    ReceivedOn  string    `json:"receivedOn"`  // "2026-09-03", date only, editable
    Basis       Basis     `json:"basis"`       // "rent" | "purchase"
    Supplier    string    `json:"supplier"`    // "" allowed
    ChallanNo   string    `json:"challanNo"`   // "" allowed
    ReceivedBy  string    `json:"receivedBy"`  // "" allowed; defaults to on-duty name
    RecordedAt  time.Time `json:"recordedAt"`
    RecordedBy  string    `json:"recordedBy"`  // on-duty staff name at time of entry
    Changes     []Change  `json:"changes,omitempty"`
    Deleted     *Deletion `json:"deleted,omitempty"`
}

type Basis string
const (
    Rent     Basis = "rent"
    Purchase Basis = "purchase"
)

type Issue struct {
    ID                  string    `json:"id"`   // "ISS-0001"
    ProductID           string    `json:"productId"`
    Quantity            int       `json:"quantity"`  // >= 1
    TakerName           string    `json:"takerName"`
    TakerDepartment     string    `json:"takerDepartment"`
    TakerMobile         string    `json:"takerMobile"`
    PersonInchargeName  string    `json:"personInchargeName"`
    PersonInchargeMobile string   `json:"personInchargeMobile"`
    IssuedAt            time.Time `json:"issuedAt"`   // auto-filled, editable
    RecordedAt          time.Time `json:"recordedAt"` // never editable
    Changes             []Change  `json:"changes,omitempty"`
    Deleted             *Deletion `json:"deleted,omitempty"`
}

type Return struct {
    ID             string       `json:"id"`   // "RET-0001"
    ProductID      string       `json:"productId"`
    Allocations    []Allocation `json:"allocations"` // >= 1 entry
    ReturnerName   string       `json:"returnerName"`
    ReturnerMobile string       `json:"returnerMobile"`
    TakenBackBy    string       `json:"takenBackBy"`  // on-duty staff name
    ReturnedAt     time.Time    `json:"returnedAt"`   // auto-filled, editable
    RecordedAt     time.Time    `json:"recordedAt"`
    ShortQuantity  int          `json:"shortQuantity"`  // 0 when nothing was short
    ShortDisposition Disposition `json:"shortDisposition,omitempty"`
    Remark         string       `json:"remark,omitempty"`
    Changes        []Change     `json:"changes,omitempty"`
    Deleted        *Deletion    `json:"deleted,omitempty"`
}

type Allocation struct {
    IssueID  string `json:"issueId"`
    Quantity int    `json:"quantity"` // >= 1
}

type Disposition string
const (
    ExpectedBack Disposition = "expected"     // "Still expected back"
    WontComeBack Disposition = "wont_return"  // "Won't come back — broken or lost"
)
```

`Return.Quantity()` is a method returning the sum of `Allocations[i].Quantity`. It is
not stored — the allocations are the truth.

**A shortfall is an annotation, never a stock movement.** `ShortQuantity` records how
many were missing at the moment of the return, and `ShortDisposition` + `Remark`
record what the desk was told. Short items stay outstanding against the taker,
whichever disposition was chosen. No code anywhere subtracts `ShortQuantity` from
anything.

**A deleted record stays in the file.** `Deleted` is a tombstone, not a removal: the
row keeps its ID, so a return's `Allocation.IssueID` can never dangle and the deletion
itself stays readable. **Every function in `03-stock-arithmetic.spec.md` skips records
whose `Deleted` is non-nil** — that rule is stated there as a contract requirement
because it has to hold in every one of them. `Changes` is append-only and is never
rewritten or trimmed.

### Identifiers

`internal/register/ids.go`:

```go
func (r *Register) NextID(prefix string) string
```

Returns `prefix + "-" + zero-padded-4-digit`, one higher than the highest existing
numeric suffix with that prefix across the matching slice. Prefixes: `PRD`, `STF`,
`INW`, `ISS`, `RET`. Padding widens past 9999 (`INW-10000`). Identifiers are never
typed or seen by the person at the desk.

### Field conventions

- **Quantities are `int`.** Never a float. Never negative. Zero is refused everywhere
  a quantity is entered.
- **Timestamps** serialise as RFC 3339 with the machine's local offset, e.g.
  `"2026-09-03T10:42:00+05:30"`. `ReceivedOn` is the only date-only field and is a
  `string` in `YYYY-MM-DD` form, so the file stays readable and no timezone
  arithmetic can shift a delivery date across midnight.
- **Names are stored exactly as entered**, with leading and trailing whitespace
  trimmed and internal runs of whitespace collapsed to one space. This trim/collapse
  is the function `register.CleanName(string) string` and every free-text name field
  passes through it before storage.
- **Comparison key.** `register.FoldKey(s string) string` is
  `strings.ToLower(CleanName(s))`. It is the one function used everywhere two names
  must be treated as the same name — product duplicates and staff duplicates.
- **Mobile key.** `register.MobileKey(s string) string` keeps only the digits of a
  mobile number, so `98861 40023`, `9886140023` and `98861-40023` are one number. It is
  never used for display; the number is shown exactly as it was typed.
- **A person holding stock is identified by name and mobile together** — see
  `03-stock-arithmetic.spec.md`. Neither alone.
- **Product names never come from free text.** Only `06-products.spec.md` may append
  to `Products`.
- **`CreatedBy` is who was on duty when the row was added**, stored as a name, set by
  `POST /product/new` (`06-products.spec.md`) and `POST /shift/person`
  (`05-shift-and-people.spec.md`). It is **empty for the first staff member on a fresh
  register**, because nobody is on duty until somebody has been added and tapped. No
  placeholder name is substituted. These two fields exist so
  `12-activity-log.spec.md` can say who added a product or a person; nothing else reads
  them, and no arithmetic touches them.

### File format

- Path: `store-register.json`, in the directory of the running executable (resolved
  from `os.Executable()`, never the working directory).
- Encoded with `json.MarshalIndent(reg, "", "  ")` plus a trailing newline, so the
  file diffs and reads cleanly.
- Key order is Go struct order, which is the order given above.

## The walkthrough fixture

`internal/register/fixture.go` exposes:

```go
func WalkthroughT0() *Register     // the register as the walkthrough's home screen shows it
func MustTime(s string) time.Time  // RFC3339 helper for tests
```

Later timepoints are built by tests by appending to `WalkthroughT0()`. Named
timepoints, used by every other spec:

| Timepoint | Clock (`Asia/Kolkata`, `+05:30`) | Register state |
|---|---|---|
| **T0** | 2026-09-03 10:00 | `WalkthroughT0()` — the Home screen |
| **T1** | 2026-09-03 10:42 | T0 + `INW-0007` (500 chairs from Sharma Tent House) |
| **T2** | 2026-09-03 14:18 | T1 + `ISS-0008` (10 chairs to Ravi Menon) |
| **T3** | 2026-09-03 18:05 | T2 + `RET-0001` (45 chairs back, 5 short) |
| **T4** | 2026-09-03 18:10 | same records as T3; the Suppliers view is read at this clock |

All fixture times are `+05:30`, built with `time.FixedZone("IST", 5*3600+30*60)` and
never `time.LoadLocation` — a `CGO_ENABLED=0 GOOS=windows` binary has no tzdata unless
`time/tzdata` is imported, and a fixed zone keeps the tests deterministic wherever the
developer's machine happens to be. 3 September 2026 is a Thursday, matching the screen
subtitles in the walkthrough.

### T0 contents

Products: `PRD-0001` Chairs, `PRD-0002` Round tables, `PRD-0003` Water drums (20L),
`PRD-0004` Extension boards, `PRD-0005` Charcoal sacks — all `CreatedAt`
`2026-09-01T08:00:00+05:30` and all `CreatedBy` `Suresh Kumar`.

Staff:

| ID | Name | Mobile | CreatedAt | CreatedBy |
|---|---|---|---|---|
| STF-0001 | Suresh Kumar | 98450 22117 | 2026-09-01T07:30:00+05:30 | *(empty — the first person, nobody on duty yet)* |
| STF-0002 | Anita Rao | 99001 34562 | 2026-09-01T07:35:00+05:30 | Suresh Kumar |
| STF-0003 | Imran Sheikh | 90080 77213 | 2026-09-01T07:40:00+05:30 | Suresh Kumar |

`OnDutyStaffID` is `STF-0001` and `ShiftStartedAt` is `2026-09-03T08:00:00+05:30`.

Inwards. `RecordedBy` equals `ReceivedBy` on every fixture inward — the person who took
the delivery is the person who typed it in:

| ID | Product | Qty | Basis | Supplier | Challan | ReceivedOn | RecordedAt | ReceivedBy / RecordedBy |
|---|---|---|---|---|---|---|---|---|
| INW-0001 | Chairs | 390 | rent | Sharma Tent House | STH/4390 | 2026-09-01 | 09-01T09:15 | Suresh Kumar |
| INW-0002 | Chairs | 310 | purchase | *(blank)* | *(blank)* | 2026-09-02 | 09-02T08:30 | Suresh Kumar |
| INW-0003 | Round tables | 60 | rent | Sharma Tent House | STH/4390 | 2026-09-01 | 09-01T09:20 | Suresh Kumar |
| INW-0004 | Water drums (20L) | 40 | purchase | *(blank)* | *(blank)* | 2026-09-01 | 09-01T11:00 | Anita Rao |
| INW-0005 | Extension boards | 25 | rent | Gupta Electricals | GE/118 | 2026-09-02 | 09-02T09:00 | Imran Sheikh |
| INW-0006 | Charcoal sacks | 12 | purchase | *(blank)* | *(blank)* | 2026-09-02 | 09-02T10:00 | Anita Rao |

Issues:

| ID | Product | Qty | Taker | Department | Mobile | Person incharge | IssuedAt |
|---|---|---|---|---|---|---|---|
| ISS-0001 | Chairs | 150 | Lakshmi Iyer | Kitchen | 99860 11204 | Suresh Kumar / 98450 22117 | 09-01T08:30 |
| ISS-0002 | Chairs | 120 | Joseph D'Cruz | Stage & Sound | 90350 66471 | Imran Sheikh / 90080 77213 | 09-01T09:15 |
| ISS-0003 | Chairs | 40 | Ravi Menon | Catering | 98861 40023 | Suresh Kumar / 98450 22117 | 09-03T09:40 |
| ISS-0004 | Round tables | 10 | Lakshmi Iyer | Kitchen | 99860 11204 | Anita Rao / 99001 34562 | 09-02T09:50 |
| ISS-0005 | Round tables | 2 | Ravi Menon | Catering | 98861 40023 | Suresh Kumar / 98450 22117 | 09-03T09:40 |
| ISS-0006 | Water drums (20L) | 5 | Farida Begum | Registration | 98455 30918 | Anita Rao / 99001 34562 | 09-02T12:15 |
| ISS-0007 | Extension boards | 25 | Joseph D'Cruz | Stage & Sound | 90350 66471 | Imran Sheikh / 90080 77213 | 09-01T09:45 |

`RecordedAt` equals `IssuedAt` for every fixture issue. Returns: empty.

This reproduces the walkthrough's home table exactly:

| Product | Came in | Out | On hand |
|---|---|---|---|
| Chairs | 700 | 310 | 390 |
| Round tables | 60 | 12 | 48 |
| Water drums (20L) | 40 | 5 | 35 |
| Extension boards | 25 | 25 | 0 |
| Charcoal sacks | 12 | 0 | 12 |

Chairs are split 390 rent from Sharma Tent House + 310 purchased with no supplier
because that is the only split under which both walkthrough screens are true at once:
the home table says 700 came in and 390 are on hand, and the Suppliers table says
Sharma was the source of 890 chairs once the 500 arrive (390 + 500 = 890). See the
Open list — this is a reading of the mockup, not something it states.

No fixture record carries a `Changes` entry or a `Deleted` tombstone. Corrections are
exercised by `11-corrections.spec.md`, which applies them to copies of these
timepoints.

Ravi Menon, Suresh Kumar, Anita Rao, Imran Sheikh, Sharma Tent House, Gupta
Electricals, the products and every quantity above come from the walkthrough. Lakshmi
Iyer, Joseph D'Cruz and Farida Begum are fixture-only names invented to make the 310
chairs out and the "out over 2 days" count land on the walkthrough's figures; they
carry no design meaning.

### T1 — INW-0007

Chairs, 500, `rent`, Sharma Tent House, challan `STH/4471`, `ReceivedOn`
`2026-09-03`, `ReceivedBy` and `RecordedBy` Suresh Kumar, `RecordedAt`
`2026-09-03T10:42:00+05:30`.

### T2 — ISS-0008

Chairs, 10, Ravi Menon / Catering / `98861 40023`, person incharge Anita Rao /
`99001 34562`, `IssuedAt` and `RecordedAt` `2026-09-03T14:18:00+05:30`.

### T3 — RET-0001

Product Chairs. Allocations `[{ISS-0003, 40}, {ISS-0008, 5}]` — 45 back.
`ReturnerName` Ravi Menon, `ReturnerMobile` `98861 40023`, `TakenBackBy` Imran Sheikh,
`ReturnedAt` and `RecordedAt` `2026-09-03T18:05:00+05:30`. `ShortQuantity` 5,
`ShortDisposition` `wont_return`, `Remark`
`5 chairs broke during setup near the stage. Ravi informed.`

## Files to create or modify

- `/home/asim/Projects/inventory-management/go.mod` — `module storeregister`, `go 1.27`
- `/home/asim/Projects/inventory-management/internal/register/model.go`
- `/home/asim/Projects/inventory-management/internal/register/ids.go`
- `/home/asim/Projects/inventory-management/internal/register/fixture.go`
- `/home/asim/Projects/inventory-management/internal/register/ids_test.go`
- `/home/asim/Projects/inventory-management/internal/register/fixture_test.go`
- `/home/asim/Projects/inventory-management/internal/register/model_test.go`

## Required tests

`TestNextIDContinuesFromHighest` — given `WalkthroughT0()`, `NextID("INW")` returns
`"INW-0007"`, `NextID("ISS")` returns `"ISS-0008"`, `NextID("RET")` returns
`"RET-0001"`, `NextID("PRD")` returns `"PRD-0006"`.

`TestNextIDOnEmptyRegister` — on `&Register{SchemaVersion: 1}`, `NextID("PRD")`
returns `"PRD-0001"`.

`TestNextIDPastFourDigits` — a register whose last inward is `INW-9999` yields
`"INW-10000"`.

`TestCleanName` — table driven:
`"  Ravi  Menon "` → `"Ravi Menon"`; `"Sharma Tent House"` → unchanged;
`"Water drums (20L)"` → unchanged; `"Chairs\t"` → `"Chairs"`; `"   "` → `""`.

`TestFoldKey` — `"Chairs"`, `"chairs"`, `"CHAIRS"`, `"  chairs  "` and `"Chairs\t"` all
give `"chairs"`; `"Ravi  Menon"` and `" ravi menon "` both give `"ravi menon"`;
`"Water drums (20L)"` gives `"water drums (20l)"`.

`TestMobileKey` — table driven: `"98861 40023"`, `"9886140023"`, `"98861-40023"` and
`" 98861 40023 "` all give `"9886140023"`; `""` and `"   "` give `""`;
`"+91 98861 40023"` gives `"919886140023"`, which is deliberately **not** the same key —
see Open.

`TestReturnQuantitySumsAllocations` — `RET-0001` from T3 reports `Quantity() == 45`.

`TestFixtureT0Totals` — for each product in `WalkthroughT0()`, the sum of inward
quantities and the sum of issue quantities equal the "Came in" and "Out" columns of
the table above: Chairs 700/310, Round tables 60/12, Water drums (20L) 40/5,
Extension boards 25/25, Charcoal sacks 12/0.

`TestFixtureCarriesProvenance` — in `WalkthroughT0()`: every `Product` has a non-zero
`CreatedAt` and `CreatedBy == "Suresh Kumar"`; every `Inward` has a non-empty
`RecordedBy` equal to its `ReceivedBy`; `STF-0002` and `STF-0003` have
`CreatedBy == "Suresh Kumar"` while `STF-0001` has `CreatedBy == ""`, and all three have
distinct `CreatedAt` values in ascending ID order. The empty `CreatedBy` on the first
staff member is deliberate and must not be filled in.

`TestFixtureReferentialIntegrity` — every `Inward.ProductID`, `Issue.ProductID` and
`Return.ProductID` in T0, T1, T2 and T3 matches a `Product.ID`; every
`Allocation.IssueID` matches an `Issue.ID`; `OnDutyStaffID` matches a `Staff.ID`; no
duplicate IDs in any slice.

`TestFixtureRoundTripsThroughJSON` — marshal `WalkthroughT0()` with
`json.MarshalIndent(v, "", "  ")`, unmarshal, and `reflect.DeepEqual` the result to
the original. Also assert the encoded bytes contain
`"receivedOn": "2026-09-01"` and `"issuedAt": "2026-09-03T09:40:00+05:30"`.

`TestEmptyRegisterEncodesEmptyArrays` — a new register encodes with
`"products": []` and not `"products": null`.

`TestUntouchedRecordsCarryNoAuditKeys` — the encoded `WalkthroughT0()` contains neither
`"changes"` nor `"deleted"` anywhere: `omitempty` keeps the file readable for the
overwhelming majority of records, which are never corrected.

`TestCorrectedRecordRoundTrips` — a copy of T1 whose `INW-0007` carries
`Changes: [{At: 2026-09-03T10:45:00+05:30, By: "Suresh Kumar", Field: "quantity",
Label: "How many", From: "500", To: "50"}]` and whose `INW-0002` carries
`Deleted: {At: ..., By: "Suresh Kumar", Reason: "Entered twice by mistake."}`
survives marshal/unmarshal `reflect.DeepEqual`, and the encoded bytes contain
`"from": "500"` and `"reason": "Entered twice by mistake."`

## Acceptance criteria

1. `go build ./...` succeeds with zero third-party imports: `go list -deps ./... | grep -v '^storeregister' | grep '\.' ` prints nothing.
2. `go test ./internal/register/` passes, and all fourteen tests above exist by the names given.
3. `grep -c 'float' internal/register/model.go` returns 0.
4. `go doc storeregister/internal/register Register` lists the original eight inventory
   fields above plus `Disposals` and encrypted `Finance` added by specs 20 and 17; it
   contains no plaintext financial collection.
5. Marshalling `WalkthroughT0()` and grepping for `"quantity": 890` returns nothing — 890 is never stored, only computed.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestNextID|TestCleanName|TestFoldKey|TestMobileKey|TestReturn|TestFixture|TestEmpty|TestUntouched|TestCorrected' -v
grep -n 'CreatedBy' internal/register/model.go   # must show it on Product and on Staff
go vet ./internal/register/
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

## Open

1. **Chairs split 390 rent / 310 purchase at T0.** Derived, not stated. It is the only
   split that satisfies both mockup screens. Confirm, or give the real composition of
   the 700.
2. **A product with both rent and purchase inwards** (Chairs, above) gets one
   Rent/Purchase pill in the stock table. Recommend: show `Rent` if any inward for
   that product is `rent`, else `Purchase`. Confirm.
3. **Mobile numbers written with a country code.** `+91 98861 40023` keys as
   `919886140023` and is therefore a different person from `9886140023`. Stripping a
   leading `91` would be a guess about number length. Recommend leaving it, and letting
   the picker (`08-issue.spec.md`) reuse the existing spelling so the second form is
   rarely typed.
4. **Persisting the on-duty person.** Storing `OnDutyStaffID` in the file means a
   restart keeps the shift; not storing it means re-tapping a name. Recommend
   storing, as specified. Confirm.
