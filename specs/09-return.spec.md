# Spec: Someone Is Returning

## Objective

Fifty chairs went out to Ravi across two separate issues; forty-five come back in one
trip at six in the evening. Imran, who was not there when either issue was made, types
`98861`, sees everything Ravi is still holding, taps the chairs, and the box is already
filled in with 50. He changes it to 45 because that is what he counted. The screen tells
him five are short, makes him say what happened to them, and keeps those five against
Ravi's name.

The person at the desk is never asked to know how many were taken.

## Context

- Owns `internal/web/return.go`, `internal/web/templates/return.html`, tests.
- Depends on `01` (`Return`, `Allocation`, `Disposition`), `03` (`FindPeople`,
  `OutstandingForPerson`, `CheckReturn`), `05` (on-duty person), `07` (`productWord`).
- Routes: `GET, POST /return/new`. `GET /api/people` and the person picker are
  specified in `08-issue.spec.md` and reused here unchanged — one finder, one
  behaviour, one set of tests.

## Contract

### `GET /return/new`

Chrome title `Someone is returning`. Heading `Someone is returning`. Sub-heading
`longstamp(now)` — `Thursday, 3 September · 6:05 pm`. No tabs.

**Step 1 — find the person.** The person picker from `08-issue.spec.md`, labelled
`Find the person`, autofocused, hint `Search by name, mobile or department.` Results
update as they type, each row reading `<Name> · <Mobile> · <Department>` so two people
who share a name are told apart by the number. When exactly one person matches, that
person is selected automatically. The `+ New person named …` row is not offered here —
somebody who has never taken anything has nothing to bring back. When the search
matches nobody: `Nobody by that name is holding anything.`

**Step 2 — what they are holding.** Above the lines, a `lab` reading
`<Name> · <Department> · <Mobile> — still holding`:
`Ravi Menon · Catering · 98861 40023 — still holding`

Then one `outrow` per outstanding line, ordered oldest issue first:

```
<strong>Chairs</strong>
Issued 9:40 am by Suresh Kumar · 40 taken, 0 back      40 out
```

The middle line is exactly
`Issued <clock(IssuedAt)> by <PersonInchargeName> · <Taken> taken, <Back> back` and the
right-hand figure is `<Out> out`.

**Selection is by product.** Tapping any row selects every row of that product for
that person and marks them all `outrow pick` — a chair is a chair, and the man is
handing back chairs, not handing back issue records. Rows of other products stay
unselected. Only one product may be returned per entry; tapping a different product
moves the selection.

**Step 3 — the quantity and the people.**

| Label | Control | Default |
|---|---|---|
| `<Product name> coming back` — `Chairs coming back` | `number`, `min=1`, `max=<total out for that product>`, autofocused when a product is picked | the full outstanding total, **50** |
| `Who is handing it back` | text, hint `Change it if somebody else brought it.` | the taker's name, `Ravi Menon` |
| `Their mobile` | text | the taker's mobile |
| `Time returned` | `datetime-local`, editable (book columns 12–13) | now |
| `Taken back by` | read-only `<On-duty name> (you)` | `Imran Sheikh (you)` |

Under the quantity, when it is less than the outstanding total, a `hint bad`:
`<short> <Product name> missing. <Taker full name> still has them.` →
`5 chairs missing. Ravi Menon still has them.`

**Step 4 — the shortfall block.** Appears only when short, below a dashed rule:

- `banner bad`: `<short> <Product name> are missing. Say what happened before you save.`
  → `5 chairs are missing. Say what happened before you save.`
- Field `What happened to the <short> <Product name>?` —
  `What happened to the 5 chairs?` — two `opt` buttons, `Still expected back`
  (`expected`) and `Won't come back — broken or lost` (`wont_return`). Neither is
  pre-selected.
- Field `Remark`, a textarea of at least 3 rows, hint
  `Write it the way you would say it.` **Required whenever the return is short.**

**The noun always travels with the number.** Never `the 5` on its own: by the time the
reader reaches this block the quantity field may have scrolled away, and a bare number
asks them to carry it across the screen. And the taker is named the same way every
time on this screen — full name, `Ravi Menon`, never `Ravi`.

Nothing in this block is offered when the full amount comes back.

Buttons: primary `Take back <n> <productWord>` → `Take back 45 chairs`; ghost `Cancel`
to `/stock`.

### `POST /return/new`

Fields: `productId`, `issueIds` (repeated), `quantity`, `returnerName`,
`returnerMobile`, `returnedAt`, `disposition`, `remark`.

Validation, in order; first failure re-renders at 200 with the person still found, the
product still picked, everything typed still in place, a `banner bad`, nothing written:

| Condition | Banner |
|---|---|
| no `issueIds` | `Tap the row for the thing they are bringing back.` |
| any issue unknown, or not all of the same product | `Tap the row for the thing they are bringing back.` |
| `quantity` not a whole number, or < 1 | `Type how many <Product name> are coming back.` |
| `CheckReturn` reports over-return | `<Taker full name> has <out> <Product name>. You cannot take back more than <out>.` |
| `CleanName(returnerName)` empty | `Who is handing it back? Type their name.` |
| short and `disposition` not one of the two | `Tap one: the <short> <Product name> are coming back later, or they are gone.` |
| short and `CleanName(remark)` empty | `Write what happened to the <short> <Product name>.` |
| `returnedAt` unparseable | `Type the time like this: 18:05.` |

Worked: `Ravi Menon has 50 chairs. You cannot take back more than 50.` This phrasing
survives a quantity of 1 — `Ravi Menon has 1 chairs` never occurs, because the product
name carries its own plural and the sentence never says `Only 1 chairs are out`.

**Allocation.** The returned quantity is spread across the selected issues **oldest
`IssuedAt` first**, filling each line's outstanding amount before moving to the next.
45 chairs against ISS-0003 (40 out, 09:40) and ISS-0008 (10 out, 14:18) produce
`[{ISS-0003, 40}, {ISS-0008, 5}]`, leaving 5 out on ISS-0008. Ties on `IssuedAt` break
by `IssueID` ascending. Allocations with a zero quantity are not stored.

On success, append:

```
Return{
  ID: NextID("RET"), ProductID: productId, Allocations: allocated,
  ReturnerName: CleanName(returnerName), ReturnerMobile: cleaned(returnerMobile),
  TakenBackBy: onDuty.Name, ReturnedAt: parsed, RecordedAt: now,
  ShortQuantity: short, ShortDisposition: disposition, Remark: CleanName(remark),
}
```

`ShortDisposition` and `Remark` are empty strings when nothing was short.

**The shortfall moves no stock.** The five chairs stay out against ISS-0008 and stay in
Ravi's outstanding list. Choosing `Won't come back` changes one thing only: the
`5 broken or lost` note on the Sharma Tent House chairs row. That note is a fact about
the goods, not a debt — nothing in this program tracks what is owed to a supplier. No
quantity anywhere changes because of that choice.

Then 303 to `/stock?saved=RET-000n`, where `/stock` shows a `banner ok`
`Took back <n> <Product name>. <Product name>: <OnHand> on hand.` followed, when
short, by a second `banner warn` `<Taker full name> still has <short> <Product name>.`

Worked: `Took back 45 chairs. Chairs: 925 on hand.` then
`Ravi Menon still has 5 chairs.`

**`<taker>` is always the person the stock was issued to**, read from
`Issue.TakerName` on the allocated lines — never `ReturnerName`. When Suresh Kumar
hands back chairs that Ravi Menon took, the shortfall stays against Ravi and the
sentence says Ravi. The same is true of the over-return refusal above and of the
`hint bad` under the quantity field, whose `<first name>` is the taker's.

`CheckReturn` is re-run inside the `store.Update` closure so two tabs cannot return the
same 50 chairs twice.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/return.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/return.html`
- `/home/asim/Projects/inventory-management/internal/web/return_test.go`

## Required tests

All against the T2 register, clock `2026-09-03T18:05:00+05:30`, Imran Sheikh
(`STF-0003`) on duty, unless stated.

`TestReturnFormRendersWalkthroughLabels` — `GET /return/new` contains
`Someone is returning`, `Thursday, 3 September · 6:05 pm`, `Find the person`,
`Search by name, mobile or department.`, `Taken back by`,
`Imran Sheikh <span class="sm">(you)</span>`, `Cancel`.

`TestReturnPickerHasNoNewPersonRow` — `GET /return/new?q=Meera` renders no
`+ New person named` row, and a query matching nobody renders
`Nobody by that name is holding anything.`

`TestReturnPickerSeparatesTwoPeopleWithOneName` — with both Ravi Kumars holding chairs,
`GET /return/new?q=Ravi Kumar` lists two rows, each with its own mobile, and picking one
shows only that man's lines.

`TestFindByMobileFragmentShowsRavisLines` — `GET /api/people?q=98861` returns exactly
one person, and `GET /return/new?q=98861` renders
`Ravi Menon · Catering · 98861 40023 — still holding` and three rows, in this order:
1. `Chairs` / `Issued 9:40 am by Suresh Kumar · 40 taken, 0 back` / `40 out`
2. `Chairs` / `Issued 2:18 pm by Anita Rao · 10 taken, 0 back` / `10 out`
3. `Round tables` / `Issued 9:40 am by Suresh Kumar · 2 taken, 0 back` / `2 out`

`TestPickingChairsSelectsBothChairLines` — `GET /return/new?q=98861&productId=PRD-0001`
renders both chair rows with class `outrow pick`, the round tables row without it, the
quantity field labelled `Chairs coming back` with `value="50"` and `max="50"`, and the
button `Take back 50 chairs`.

`TestPickingTablesDefaultsToTwo` — `productId=PRD-0002` gives `value="2"`, `max="2"`,
label `Round tables coming back`, button `Take back 2 round tables`.

`TestShortfallHintAtFortyFive` — the form re-rendered with `quantity=45` contains
`5 chairs missing. Ravi Menon still has them.` and
`5 chairs are missing. Say what happened before you save.` and
`What happened to the 5 chairs?` and `Still expected back` and
`Won't come back — broken or lost` and
`Write it the way you would say it.`

`TestNoShortfallBlockOnFullReturn` — re-rendered with `quantity=50`, none of the
strings in the previous test appear.

`TestReturn45Of50` — `POST /return/new` with
`productId=PRD-0001&issueIds=ISS-0003&issueIds=ISS-0008&quantity=45&returnerName=Ravi Menon&returnerMobile=98861 40023&returnedAt=2026-09-03T18:05&disposition=wont_return&remark=5 chairs broke during setup near the stage. Ravi informed.`:
- 303 to `/stock?saved=RET-0001`;
- the stored return equals RET-0001 in `01-data-model.spec.md` exactly, including
  `Allocations: [{ISS-0003,40},{ISS-0008,5}]`, `ShortQuantity: 5`, and
  `TakenBackBy: "Imran Sheikh"`;
- `OnHand(PRD-0001) == 925`;
- `OutstandingForPerson(reg, "Ravi Menon", "98861 40023")` is two lines: 5 chairs on
  ISS-0008 and 2 round tables on ISS-0005;
- the redirect target contains `Took back 45 chairs. Chairs: 925 on hand.`
  and `Ravi Menon still has 5 chairs.`

`TestReturnAllocatesOldestFirst` — after the post above, `ISS-0003` has 0 outstanding
and `ISS-0008` has 5 — not 40 and 5 the other way round.

`TestFullReturnOf50` — the same post with `quantity=50`, no disposition, no remark:
succeeds, `OnHand == 930`, Ravi holds only the 2 round tables, and the stored return has
`ShortQuantity: 0`, `ShortDisposition: ""`, `Remark: ""`.

`TestReturnRefuses51Of50` — `quantity=51` returns 200 containing
`Ravi Menon has 50 chairs. You cannot take back more than 50.`; no return is written
and `OnHand` is still 880. "Returning more than was taken is refused outright."

`TestReturnRefusesZeroAndGarbage` — `quantity=0`, `-1`, `abc`, `4.5` each return 200
with `Type how many chairs are coming back.`

`TestShortReturnWithoutDispositionRefused` — `quantity=45` with `disposition=` and a
remark present returns 200 containing
`Tap one: the 5 chairs are coming back later, or they are gone.`; nothing is written.

`TestShortReturnWithoutRemarkRefused` — `quantity=45`, `disposition=wont_return`,
`remark=   ` returns 200 containing
`Write what happened to the 5 chairs.`; nothing is written. **A short return
can never be saved silently.**

`TestShortReturnExpectedBackKeepsSameNumbers` — the walkthrough post with
`disposition=expected` instead: `OnHand == 925`, Ravi still holds 5 chairs, and the
Sharma/Chairs `CameIn` is 890 either way; only `WontComeBack` differs (0 instead of 5),
and `/suppliers` shows `5 broken or lost` in one case and no note in the other.

`TestReturnRefusesNoSelection` — a post with no `issueIds` returns 200 with
`Tap the row for the thing they are bringing back.`

`TestReturnRefusesMixedProducts` — `issueIds=ISS-0003&issueIds=ISS-0005` (chairs and
round tables) returns 200 with `Tap the row for the thing they are bringing back.`

`TestReturnRefusesEmptyReturner` — `returnerName=  ` returns 200 with
`Who is handing it back? Type their name.`

`TestReturnByADifferentPerson` — the walkthrough post (45 of 50, short 5) with
`returnerName=Suresh Kumar&returnerMobile=98450 22117` succeeds; the stored return has
`ReturnerName: "Suresh Kumar"` while the issues remain Ravi Menon's, and Ravi is still
the person holding the outstanding 5. The redirect target contains
`Ravi Menon still has 5 chairs.` and **does not** contain
`Suresh Kumar still has`. Then a further post of 6 chairs by Suresh is
refused with `Ravi Menon has 5 chairs. You cannot take back more than 5.` — never
"out with Suresh Kumar".

`TestSecondPartialReturnAgainstSameIssue` — after the 45-chair return, post a second
return of 3 chairs against `ISS-0008` with `disposition=expected` and remark
`2 chairs still with the catering tent.` Result: `OnHand == 928`, ISS-0008 has 2
outstanding, and a third return of 3 is refused with
`Ravi Menon has 2 chairs. You cannot take back more than 2.`

`TestReturnRevalidatesAtSaveTime` — render with 50 outstanding, then mutate the store so
another return has already taken 48, then post 45: 200 with
`Ravi Menon has 2 chairs. You cannot take back more than 2.` and nothing written.

`TestReturnIsAtomicOnDisk` — after the successful 45-chair return,
`store-register.json.bak` parses with 0 returns and `store-register.json` with 1.

## Acceptance criteria

1. `go test ./internal/web/ -run 'TestReturn|TestFind|TestPicking|TestShort|TestFull|TestSecond' -count=1` passes with all twenty-three tests.
2. The success path returns 303 with `Location: /stock?saved=RET-0001`; every refusal
   returns 200 and no refusal body contains `invalid`, `error`, `nil` or `panic`.
3. `grep -n 'ShortQuantity' internal/web/return.go internal/register/arith.go` shows
   `ShortQuantity` read only for display and for the supplier note — it must never
   appear in an expression that also mentions `OnHand`, `OutWithPeople` or `StillOwed`.
4. A test asserts `Allocations` sums to the posted quantity for every successful return
   in the package.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestReturn|TestShort|TestFull|TestPicking|TestFind' -count=1 -v
go vet ./internal/web/
```

## Open

1. **`Nobody by that name is holding anything.`** — written here, not reviewed. It has
   to cover a search by mobile too, where "by that name" is slightly wrong.
2. **Whether the person handing things back should also come off the picker.**
   `Who is handing it back` is plain text defaulting to the taker, per the mockup. A
   picker there would let the desk record "Suresh brought Ravi's chairs" without
   retyping. Not in the walkthrough; not built unless asked.

Settled and recorded in `00-index.spec.md`: allocation is oldest issue first; selection
is by product, not by line; one product per return entry; the over-return sentence and
the post-save confirmations as worded above; the `Time returned` label.
