# Spec: Fixing a Wrong Entry

## Objective

Somebody types 500 where they meant 50, in the first hour, and every number for the rest
of the day is wrong. The person at the desk cannot open the file and edit it, so the
software has to let them fix it — and has to refuse the fixes that would leave the
register telling a lie. Nothing is ever rewritten quietly: a corrected entry says what
it used to say, who changed it and when, in the same plain words as everything else.

## Context

- Owns `internal/web/corrections.go`, `internal/web/templates/edit.html`, tests.
- Depends on `01-data-model.spec.md` (`Change`, `Deletion`, the `Changes` and `Deleted`
  fields), `03-stock-arithmetic.spec.md` (**`Validate`**, and the rule that every
  arithmetic function skips deleted records), `05` (on-duty person), `07` (`productWord`).
- Reached from the lists, never from an admin screen: the `Fix this` links on the
  "Stuff came in" and "Out with people" tabs (`10-views.spec.md`).
- Routes: `GET /entry/{id}/edit`, `POST /entry/{id}/edit`, `POST /entry/{id}/delete`.

## Contract

### How a correction is decided — one checker, not four formulas

**Every guard in this spec is the same guard.** A correction is applied to a deep copy
of the register, `register.Validate(copy)` is run, and if it reports a problem the
correction is refused and nothing is written. The per-field sentences below are how a
`Problem` is *phrased*; they are never how the decision is *made*. Two hand-derived
minimums that have to agree is a bug waiting to happen; one checker with a message layer
is not.

```go
func Apply(r *Register, edit Edit, by string, now time.Time) ([]Problem, error)
```

The two returns mean different things and are handled differently. A non-empty
`[]Problem` is a **refusal to show the person at the desk** — the register was not
changed, the response is 200, and the sentences below are rendered in a `banner bad`. A
non-nil `error` is a **save failure** — a bad form value or a disk problem — handled the
way every other handler handles one. Never both.

The whole thing runs inside one `store.Update` closure, so a refused correction leaves
neither the file nor the in-memory register changed (`02-persistence.spec.md`).

### `GET /entry/{id}/edit`

The record's own form, pre-filled with what it currently says, laid out exactly like the
entry screen it came from — the same labels, the same order, so nobody has to learn a
second version of a screen they already know.

Chrome title `Fix an entry`. Heading, by record type: `Fix what came in`,
`Fix what someone took`, `Fix what came back`. Sub-heading names the entry in words, not
an ID: `500 chairs from Sharma Tent House, received 3 September` /
`10 chairs to Ravi Menon, 2:18 pm` / `45 chairs back from Ravi Menon, 6:05 pm`.

The naming part of those three — everything before the comma — is the shared helper
`entryName(reg *register.Register, recordID string) string`, living in
`internal/web/corrections.go`: `500 chairs from Sharma Tent House` (or
`500 chairs that came in` when no supplier was recorded), `10 chairs to Ravi Menon`,
`45 chairs back from Ravi Menon`. This screen appends the time clause;
`12-activity-log.spec.md` uses the bare name, because its own `Time` column supplies
the clause.

Editable and not editable, by record:

| Record | Editable | Fixed |
|---|---|---|
| Inward | `How many`, `Date received`, `Rent or purchase`, `Came from`, `Challan no.` | the product; `Received by`; `RecordedAt` |
| Issue | `How many`, `Who is taking it`, `Department`, `Their mobile`, `Time taken` | the product; `Person incharge (giving it)`; `RecordedAt` |
| Return | `How many came back`, `Who is handing it back`, `Their mobile`, `Time returned`, and the shortfall block | the product; the person the stock was issued to; `Taken back by`; `RecordedAt` |

**The product cannot be changed on any record.** Moving a quantity from one pool to
another is two corrections wearing one coat, and it can make both pools wrong at once.
The screen shows the product as plain text with the sentence
`Wrong product? Delete this entry and enter it again.`

Below the form, every correction already made, oldest first, in `sm` type:
`Changed how many from 500 to 50 by Suresh Kumar, 10:45 am`.

Buttons: primary `Make it <n> <productWord>` when the quantity has been changed —
`Make it 50 chairs` — and `Save this fix` when it has not, so the button always names
what pressing it does. Ghost `Cancel`. Separated to the right, a `Delete this entry`
button in `bad` colouring that opens the delete block rather than deleting on the first
press.

### `POST /entry/{id}/edit`

For each editable field whose submitted value differs from the stored one, a `Change` is
appended:

```
Change{At: now, By: onDuty.Name, Field: "quantity", Label: "How many",
       From: "500", To: "50"}
```

`From` and `To` are rendered the way the person saw them: `500`, `Sharma Tent House`,
`3 September`, `2:18 pm`, and for rent-or-purchase the pill wording the lists already
show — `Rent` and `Purchase`, not the button's `On rent — goes back to supplier`. One
`Change` per field changed; a save that changes
nothing appends nothing and is not an error.

**Line shapes.** The both-filled phrase is spelled out per field rather than generated
by lower-casing the label, which would produce `Changed came from from Sharma Tent House
to Sharma Tent House & Sons` and `Changed rent or purchase from On rent to Purchase`:

| Field | Both filled |
|---|---|
| quantity | `Changed it from 500 chairs to 50 chairs` — the noun travels with both numbers, because this phrase is also rendered in the activity log where no form label supplies it |
| supplier | `Changed the supplier from Sharma Tent House to Sharma Tent House & Sons` |
| date received | `Changed the date from 3 September to 4 September` |
| basis | `Changed this from rent to purchase` — `this`, not `it`: in the log the phrase sits in a table column with no record heading above it to give `it` an antecedent |
| challan | `Changed the challan no. from STH/4471 to STH/4472` |
| taker, returner | `Changed who took it from Ravi Menon to Ravi Verma` |
| department | `Changed the department from Catering to Security` |
| mobile | `Changed the mobile from 98861 40023 to 98861 40024` |
| time | `Changed the time from 2:18 pm to 2:40 pm` |
| remark | `Changed the remark from ... to ...` |

Each is followed by ` by <name>, <clock>` — `Changed how many from 500 to 50 by Suresh
Kumar, 10:45 am`. The two halves are two functions in
`internal/web/corrections.go`, because `12-activity-log.spec.md` renders the phrase
without the suffix its own `Who` and `Time` columns already carry:

```go
func changePhrase(c register.Change) string  // "Changed how many from 500 to 50"
func changeLine(c register.Change) string    // changePhrase(c) + " by " + c.By + ", " + clock(c.At)
```

`changeLine` is what this screen, `/inwards` and `/out` render. There is one table of
line shapes and one implementation of it; nothing may restate them.

The `date received` row renders both sides through `shortdate`
(`04-server-and-shell.spec.md`), so `3 September` here and `3 September` in the log come
from one function.

Where one side is empty:

| Case | Line |
|---|---|
| `To` empty | `Removed the note that said: 5 chairs broke during setup. by Suresh Kumar, 10:45 am` — the removed content is named, because in the activity log this phrase is the whole of the row's explanation |
| `From` empty | `Added a note: 5 chairs broke during setup. by Suresh Kumar, 10:45 am` |

Those two use the field's `Label` lower-cased, which reads correctly because the
sentence does not repeat a preposition. A cleared remark or a cleared
disposition is a real correction and gets a real line; it is never silently dropped
because the new value happens to be an empty string.

Field-level validation is the entry screen's, unchanged — a quantity must still be a
whole number of 1 or more, a taker must still have a name, a short return must still
carry a disposition and a remark. Then the whole-register check runs.

**Refusals.** Each one names the number that is in the way and what to do about it:

| Problem | Sentence |
|---|---|
| Reducing an inward below what has gone out | `310 chairs are out with people. Take some back before you go below 310 chairs.` |
| Raising an issue above what is on hand | `Counting the 10 chairs already on this entry, you have 890 chairs. You cannot give out more than 890.` — the ceiling is `OnHand + this entry's current quantity` (880 + 10), because the 10 already on this entry are being re-spent, not spent twice |
| Reducing an issue below what has come back against it | `40 chairs have already come back. To go below 40 chairs, fix that return first.` |
| Raising a return above what that person took | `Ravi Menon took 50 chairs. You cannot put back more than 50.` |
| Reducing a return so that on-hand goes negative | `20 chairs have gone out and only 10 came in. Keep this at 10 chairs or more, or delete the issue first.` |

The numbers in these sentences are computed from the `Problem` that `Validate` returned
plus the record being edited, not from a second formula.

On success, 303 back to the tab the `Fix this` link came from (`/inwards` or `/out`,
carried in a `from` field), with a `banner ok`:
`Fixed. <what it now says>.` — worked: `Fixed to 50 chairs. Chairs: 440 on hand.`

### Editing a return's quantity — re-allocation

Changing how many came back **discards the old allocations and runs the oldest-first
allocation from scratch** over the same set of issues, exactly as a fresh return of that
number would (`09-return.spec.md`). It never adjusts the existing split in place, which
would silently produce a different answer from entering the same number twice.

Worked, from T3: RET-0001 is `[{ISS-0003, 40}, {ISS-0008, 5}]`. Edited from 45 to 40, it
becomes `[{ISS-0003, 40}]` — ISS-0008 drops out of the record entirely rather than
keeping a zero. Edited to 50, it becomes `[{ISS-0003, 40}, {ISS-0008, 10}]`.

Changing the quantity re-derives the shortfall: `ShortQuantity` is what the selected
issues had out before this return, less the new quantity. If that is now 0, the
disposition and remark are cleared and a `Change` records it. If it is now above 0 and
no disposition or remark is on the form, the save is refused with the sentences from
`09-return.spec.md`.

### `POST /entry/{id}/delete`

The delete block asks for one thing: `Why are you deleting this?`, a text field, hint
`One line is enough — "entered twice", "wrong person".` **Required.** Then a button
reading `Yes, delete these <n> <productWord>` — `Yes, delete these 500 chairs`.

The confirming button must **not** repeat the words `Delete this entry` from the button
that opened this block. A busy reader who pressed that once and saw nothing disappear
will press the identical-looking button again without reading, and the second press is
the one that acts.

Deleting sets `Deleted = &Deletion{At: now, By: onDuty.Name, Reason: reason}`. The
record stays in the file and stays visible, struck through, in the list it came from. It
counts towards nothing.

The same one-checker guard applies: apply the tombstone to a copy, run `Validate`, refuse
if it complains.

| Problem | Sentence |
|---|---|
| Deleting an inward the stock has already gone out of | `310 of these chairs are out with people. Take them back first, then delete this.` |
| Deleting an issue that has something back against it | `40 chairs have already come back. Delete that return first, then this.` |
| Deleting a return that would push on-hand negative | `20 chairs have gone out and only 10 came in. Delete the issue that took them first, then this.` |

**"Delete the return, then the issue" must work.** The allocations that block an issue's
deletion are counted from **live returns only**; a return that has itself been deleted
holds nothing. Otherwise the two records lock each other in place forever and the desk
has no way out — and that ordering is the main thing this feature exists to support.

A deleted record cannot be edited or deleted again; `GET /entry/{id}/edit` for one
returns 200 with the record shown read-only and the line
`Deleted by Suresh Kumar, 10:47 am. Enter it again if that was a mistake.`

### What corrections are not

No undo of a delete, no edit history screen, no diff view, no "restore". The audit line
sits inline on the record and that is the whole of it.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/corrections.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/edit.html`
- `/home/asim/Projects/inventory-management/internal/web/corrections_test.go`
- `/home/asim/Projects/inventory-management/internal/register/validate.go` (spec 03)

## Required tests

Suresh Kumar on duty, clock `2026-09-03T10:45:00+05:30` unless stated. Every test that
expects a refusal also asserts that the register on disk and the register in memory are
both byte-for-byte what they were before the request.

### Inward corrections

`TestFixInwardQuantity` — over T1, `POST /entry/INW-0007/edit` with `quantity=50`:
303 to `/inwards`, `OnHand(Chairs) == 440`, `CameIn(Chairs) == 750`, and the record
carries exactly one `Change`
`{Field: "quantity", Label: "How many", From: "500", To: "50", By: "Suresh Kumar",
At: 10:45}`. `/inwards` then shows
`Changed how many from 500 to 50 by Suresh Kumar, 10:45 am`.

`TestFixInwardSupplierAndChallan` — changing `supplier` to `Sharma Tent House & Sons`
and `challanNo` to `STH/4472` appends two `Change` entries, in field order, and
`SupplierRows` now shows the new supplier name with 500 chairs and the old one with 390.

`TestFixInwardChangingNothingAppendsNothing` — re-posting the form unchanged returns 303
and leaves `Changes` empty.

`TestFixInwardRefusedBelowWhatIsOut` — start from T0, delete `INW-0002` (the 310
purchased chairs) so that `INW-0001`'s 390 are the only chairs in the register and 310
of them are out. `POST /entry/INW-0001/edit` with `quantity=200` is refused with
`310 chairs are out with people. Take some back before you go below 310 chairs.`
On the same register `quantity=310` is accepted and leaves `OnHand == 0`.

`TestFixInwardBoundaryIsTheFieldRuleNotTheRegisterRule` — start from T0 with both chair
inwards live, so 700 came in and 310 are out. `INW-0001` accepts `quantity=1`
(`CameIn == 311`, `OnHand == 1`), because the other 310 chairs cover what is out.
`quantity=0` is refused by the entry screen's own field rule with
`How many came in? Type a number of 1 or more.`, not by `Validate` — a quantity of zero
is not a correction, it is a deletion, and deletion has its own button.

`TestFixInwardCannotChangeProduct` — a post carrying `productId=PRD-0002` against
`INW-0007` leaves `ProductID` as `PRD-0001`, appends no `Change`, and the rendered form
contains `Wrong product? Delete this entry and enter it again.`

### Issue corrections

`TestFixIssueQuantityUp` — over T2 (880 chairs on hand), `POST /entry/ISS-0008/edit`
with `quantity=50`: accepted, `OnHand == 840`, Ravi holds 90 chairs.

`TestFixIssueRefusedAboveOnHand` — over T2, `quantity=900` on ISS-0008 is refused with
`Counting the 10 chairs already on this entry, you have 890 chairs. You cannot give out more than 890.` — 890 being the 880 still
on hand plus the 10 this entry already accounts for. `quantity=890` is accepted and
leaves `OnHand == 0`.

`TestFixIssueRefusedBelowWhatCameBack` — over T3, `POST /entry/ISS-0003/edit` with
`quantity=30` is refused with
`40 chairs have already come back. To go below 40 chairs, fix that return first.`
`quantity=40` is accepted.

`TestFixIssueTaker` — over T2, changing `takerName` to `Ravi Menon` / `takerMobile` to
`98861 40024` moves the 10 chairs to a different person: `PeopleHolding` shows two Ravi
Menons, one with 42 out and one with 10. This is allowed without comment — people
records may be sloppy (`00-index.spec.md`).

`TestFixIssueTime` — changing `issuedAt` to `2026-09-03T13:05` appends a `Change` reading
`From: "2:18 pm", To: "1:05 pm"` and leaves `RecordedAt` untouched.

### Return corrections

`TestFixReturnQuantityDownReallocates` — over T3, `POST /entry/RET-0001/edit` with
`quantity=40`, `disposition=wont_return`, remark
`10 chairs broke during setup near the stage. Ravi informed.`: allocations become
exactly `[{ISS-0003, 40}]`, ISS-0008 is absent from them, `OnHand == 920`, Ravi holds 10
chairs, and `ShortQuantity == 10`.

`TestFixReturnQuantityUpClearsTheShortfall` — over T3, `quantity=50` with no disposition
and no remark: accepted, allocations `[{ISS-0003, 40}, {ISS-0008, 10}]`,
`ShortQuantity == 0`, `ShortDisposition == ""`, `Remark == ""`, `OnHand == 930`, and
Ravi holds no chairs. Three `Change` entries are appended — the quantity, the cleared
remark and the cleared disposition — and `/out` renders
`Removed the remark by Suresh Kumar, 10:45 am`, never a sentence ending in `to `.

`TestFixReturnRefusedAboveWhatWasTaken` — over T3, `quantity=51` is refused with
`Ravi Menon took 50 chairs. You cannot put back more than 50.`

`TestFixReturnStillDemandsARemarkWhenShort` — over T3, `quantity=40` with
`disposition=` and `remark=` is refused with the spec 09 sentences
`Tap one: the 10 chairs are coming back later, or they are gone.` and, once a
disposition is given, `Write what happened to the 10 chairs.`

`TestFixReturnReallocationMatchesAFreshEntry` — edit RET-0001 from 45 to 40, and
separately delete RET-0001 and enter a fresh 40-chair return over the same two issues.
The two registers have identical allocations. This is the test that catches an in-place
adjustment.

### Deletions

`TestDeleteInwardEnteredTwice` — over T1 with a duplicate `INW-0008` of 500 chairs,
`POST /entry/INW-0008/delete` with `reason=Entered twice by mistake.`: 303,
`OnHand(Chairs) == 890`, the record still present with its tombstone, `/inwards` showing
`Deleted by Suresh Kumar, 10:47 am — Entered twice by mistake.`

`TestDeleteRefusedWithoutAReason` — `reason=   ` returns 200 containing
`Why are you deleting this?` again with a `banner bad`, and the record is not deleted.

`TestDeleteInwardRefusedWhenStockHasGoneOut` — over T0, delete `INW-0002` (accepted,
`OnHand == 80`), then delete `INW-0001`: refused with
`310 of these chairs are out with people. Take them back first, then delete this.`

`TestDeleteIssueWithNothingBack` — over T3, delete `ISS-0005` (2 round tables, nothing
returned) with reason `Ravi never took the tables.`: accepted, round tables
`OnHand == 50`, Ravi holds only 5 chairs.

`TestDeleteIssueRefusedWhenSomethingCameBack` — over T3, delete `ISS-0003`: refused with
`40 chairs have already come back. Delete that return first, then this.`

`TestDeleteReturnThenTheIssue` — over T3: delete `RET-0001` with reason
`Wrong person, Ravi never brought these.` (accepted — `OnHand == 880`, Ravi back to 50
chairs out), then delete `ISS-0003` with reason `Entered twice.` (accepted, because the
allocations that blocked it belonged to a return that is now deleted).
**This ordering must work; it is the way out of a wrong pair of entries.**

`TestDeleteReturnRefusedWhenOnHandWouldGoNegative` — the small register from spec 03's
`TestValidateCatchesNegativeOnHand`: 10 chairs in, 10 issued, 10 returned, 10 issued
again. Deleting the return is refused with
`20 chairs have gone out and only 10 came in. Delete the issue that took them first, then this.` Editing the
same return from 10 down to 3 is refused with
`20 chairs have gone out and only 10 came in. Keep this at 10 chairs or more, or delete the issue first.`

`TestDeletedEntryCannotBeEditedAgain` — `GET /entry/INW-0008/edit` on a deleted record
returns 200 containing
`Deleted by Suresh Kumar, 10:47 am. Enter it again if that was a mistake.`
and no save button; `POST` to either route returns 200 with the same line and
changes nothing.

### The register never ends up wrong

`TestValidateIsCleanAfterEverySuccessfulCorrection` — a table of every accepted
correction in this spec; after each, `register.Validate(reg)` returns no problems.

`TestRefusedCorrectionWritesNothing` — a table of every refusal above; after each, the
main file and the `.bak` are unchanged and a fresh `store.Open` of the directory returns
a register `reflect.DeepEqual` to the one before the request.

`TestCorrectionsAccumulate` — three successive edits to `INW-0007` (500 → 50 → 55 → 55
with a changed challan) leave three `Change` entries in chronological order, and the
first still reads `From: "500", To: "50"`.

## Acceptance criteria

1. `go test ./internal/web/ -run 'TestFix|TestDelete|TestCorrections|TestValidateIsClean|TestRefused' -count=1` passes with all twenty-seven tests.
2. Every refusal returns 200; no refusal body contains `invalid`, `error`, `nil`,
   `panic` or a record ID.
3. **The one-checker rule holds mechanically:**
   `grep -c 'Validate(' internal/web/corrections.go` is at least 2 (edit and delete),
   and `grep -nE 'OnHand\(|OutWithPeople\(|CameIn\(' internal/web/corrections.go` shows
   those calls only inside message-formatting functions — never inside an `if` that
   decides whether to save. A reviewer can check this by reading one file.
4. `grep -n 'ProductID' internal/web/corrections.go` shows it read but never assigned.
5. `go test ./internal/web/ -race -count=1` passes.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestFix|TestDelete|TestCorrections' -count=1 -v
go test ./internal/register/ -run TestValidate -count=1 -v
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

## Settled

1. **A reason is required on delete, and not on an edit.** Settled as specified. A
   deleted entry leaves nothing behind to explain itself, so the reason is the only
   trace; an edit already shows its own before-and-after, which explains itself. This
   also keeps the friction where it belongs — deletes are rare, edits are not, and the
   user's standing instruction is that speed at the desk wins wherever correctness does
   not require otherwise.
2. **No un-delete.** Re-entering the record is one press more and leaves a truer
   history than resurrection would.
3. **`Received by`, `Person incharge (giving it)` and `Taken back by` are not correctable.** They
   record who was standing there at the time, and the person doing the correcting is
   usually somebody else. Correcting them would let one person quietly reassign another
   person's name to an entry, which is the one thing attribution must not permit.

## Open

1. **The wording of the five refusal sentences, the delete prompt and the audit line.**
   With `plain-language-reviewer`. Nothing else waits on it — the sentences are asserted
   verbatim in tests, so a reword touches this spec and its tests and nothing else.
