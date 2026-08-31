# Spec: Someone Is Taking

## Objective

Ravi comes to the desk for 10 chairs. Anita, who started at 2pm and has never met him,
does not need to know anything: the screen tells her there are 890 on hand, refuses any
number larger than that, fills in Ravi's department and mobile from the last time he
took something, and warns her — in amber, unasked — that he is already holding 40
chairs and 2 round tables from earlier today.

## Context

- Owns `internal/web/issue.go`, `internal/web/templates/issue.html`, tests.
- Depends on `01` (`Issue`), `03` (`OnHand`, `CheckIssue`, `OutstandingForPerson`,
  `PeopleHolding`), `05` (on-duty person), `06` (product picker),
  `07` (`productWord`).
- Routes: `GET, POST /issue/new`, `GET /api/people`, optional query
  `?productId=PRD-0001` when arriving from an `Issue` button on the stock table.

## Contract

### `GET /api/people?q=<text>` — the one person-finder

Owned here because the issue screen is the first to need it; the returning screen
(`09`), the "Out with people" view (`10`) and the "Who did what" view (`12`) call the
same endpoint and render the same partial. `12-activity-log.spec.md` adds one optional
parameter, `scope=log`, which swaps `FindPeople` for `FindPeopleInLog` — a different
population, the same JSON shape, the same partial, the same JavaScript. Without it the
behaviour below is unchanged. `application/json`, the output of
`register.FindPeople(reg, q)` — for each
person their name, mobile, department, total out, and their outstanding lines with
`issueId`, `productId`, `productName`, `taken`, `back`, `out`, `issuedAt`, `issuedBy`.
An empty `q` returns everyone holding something.

### The person picker

One input. The person at the desk types a name **or** a mobile number and the same list
appears either way, because nobody should have to work out which box a number goes in.
Each suggestion row is:

```
Ravi Kumar · 98861 40023 · Catering
Ravi Kumar · 97740 11298 · Security
+ New person named Ravi Kumar
```

The mobile is in the middle because it is the thing that tells two people with one name
apart. Tapping a row fills the name, mobile and department fields. The
`+ New person named <what was typed>` row is always last and always present — **except
in `scope=log`** (`12-activity-log.spec.md`), where the picker filters a read-only list
and nobody is created from it.

**It offers; it never insists.** No confirmation step, no "did you mean", no warning
banner, no blocked submit. Somebody who ignores the list and keeps typing is never
stopped, and a name that already exists in a different spelling is saved without
comment. Matching for the purpose of *showing* the list is loose — case-insensitive
substring on the name, digits-only substring on the mobile, so the spacing in
`98861 40023` never matters. Identity for the purpose of *storing* stays name and
mobile together (`03-stock-arithmetic.spec.md`).

**This is deliberately the same shape as the product picker in `06-products.spec.md`,
with the opposite enforcement, and the difference is the point.** The product picker
refuses a near-duplicate: a split product silently halves the on-hand figure and nobody
notices until the count is already wrong. The person picker merely offers: a duplicate
person is a cosmetic problem that the staff can see and sort out between themselves,
and stopping a queue to resolve it costs more than it saves.

**One silent kindness.** When what was typed matches exactly one existing person and
the mobile field is still empty, that person's mobile and department are filled in.
This is the common case — the same man coming back for more chairs — and it keeps him
from becoming two rows through nothing but a hurried second entry.

### `GET /issue/new`

Chrome title `Someone is taking`. Heading `Someone is taking` — the heading reuses the
chrome title rather than switching to warehouse register (`Issue stock`) halfway down
the same screen. Sub-heading `longstamp(now)` —
`Thursday, 3 September · 2:18 pm`. No tabs.

| Label | Control | Behaviour |
|---|---|---|
| `Product` | picker from 06, `mode=instock`, **no** add-new row; pre-filled and shown as plain text when `?productId=` was supplied | hint below, green (`hint good`): `<OnHand> on hand right now` |
| `How many` | `number`, `min=1`, `max=<OnHand>`, autofocused | hint: `Most you can issue is <OnHand>.` |
| `Who is taking it` | the person picker above | when the typed name matches exactly one person, department and mobile fill in and the hint reads `This person has taken things before. Their details are filled in.` |
| `Department` | text | editable; pre-filled from the picked person's last issue |
| `Their mobile` | text | editable; pre-filled from the picked person's last issue |
| `Time taken` | `datetime-local` | defaults to now, editable (book columns 11–12) |
| `Person incharge (giving it)` | read-only `<On-duty name> (you)` | — |
| `Your mobile` | read-only, on-duty mobile | — |

`Product`/`How many` share a `row2`; `Department`/`Their mobile` share a `row2`;
`Person incharge`/`Your mobile` share a `row2`.

**The amber banner.** Rendered directly above the buttons, `banner warn`, whenever the
named taker has anything outstanding:

`<Name> is already holding <list> from earlier today.`

- `<list>` is one clause per product, `<qty> <productWord(name)>`, joined with commas
  and a final ` and `: `40 chairs and 2 round tables`.
- Products in the list are ordered by descending quantity, ties alphabetical.
- The trailing ` from earlier today` appears only when every one of that person's
  outstanding issues was issued on the same calendar day as `now`; otherwise the
  sentence ends after the list.
- The person is matched on name **and** mobile, so the banner never shows one Ravi
  Kumar's holdings while stock is being given to the other.
- The banner re-renders as the name or mobile field changes (a fetch to
  `/api/people?q=`), and is present in the server-rendered HTML whenever the posted
  form is re-rendered.
- It is information, not an obstacle: there is nothing to dismiss and nothing to
  confirm.

Buttons: primary `Issue <n> <productWord> to <first name>`; ghost `Cancel` to `/stock`.
`<first name>` is the first whitespace-separated word of the taker's name — `Ravi`.
Before a product, quantity or name is chosen the label reads `Issue`.

### `POST /issue/new`

Fields: `productId`, `quantity`, `takerName`, `takerDepartment`, `takerMobile`,
`issuedAt`.

`personInchargeName` and `personInchargeMobile` are **never read from the form**; they
come from the on-duty staff record.

Validation, in order; first failure re-renders at 200 with everything typed still in
place, a `banner bad`, and nothing written:

| Condition | Banner |
|---|---|
| `productId` empty or unknown | `Pick the product from the list.` |
| `quantity` not a whole number, or < 1 | `Type how many <Product name> they are taking.` |
| `CheckIssue` reports over-issue, `OnHand > 0` | `You have <OnHand> <Product name>. You cannot give out more than <OnHand>.` |
| `CheckIssue` reports over-issue, `OnHand == 0` | `There are no <Product name> left.` |
| `CleanName(takerName)` empty | `Who is taking it? Type their name.` |
| `issuedAt` unparseable | `Type the time like this: 14:18.` |

Worked: `You have 890 chairs. You cannot give out more than 890.` The product's own
name carries its plural, so a quantity of 1 never produces `Only 1 chairs are on hand`
— the failure the no-pluralisation rule (index decision 10) would otherwise cause.

On success, append:

```
Issue{
  ID: NextID("ISS"), ProductID: productId, Quantity: n,
  TakerName: CleanName(takerName), TakerDepartment: CleanName(takerDepartment),
  TakerMobile: cleaned(takerMobile),
  PersonInchargeName: onDuty.Name, PersonInchargeMobile: onDuty.Mobile,
  IssuedAt: parsed, RecordedAt: now,
}
```

then 303 to `/stock?saved=ISS-000n`, where `/stock` shows a `banner ok`:
`Gave <n> <Product name> to <Taker full name>. <Product name>: <OnHand> on hand.`

Worked: `Gave 10 chairs to Ravi Menon. Chairs: 880 on hand.`

The over-issue check is re-run inside the `store.Update` callback, against the register
as it is at that instant — not against the number rendered on the page — so two tabs
cannot both spend the last 10 chairs.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/issue.go`
- `/home/asim/Projects/inventory-management/internal/web/people.go` — `/api/people` and
  the picker, shared with 09 and 10
- `/home/asim/Projects/inventory-management/internal/web/templates/issue.html`
- `/home/asim/Projects/inventory-management/internal/web/templates/person-picker.html`
- `/home/asim/Projects/inventory-management/internal/web/static/person-picker.js`
- `/home/asim/Projects/inventory-management/internal/web/issue_test.go`
- `/home/asim/Projects/inventory-management/internal/web/people_test.go`

## Required tests

All against the T1 register (890 chairs on hand) unless stated, clock
`2026-09-03T14:18:00+05:30`, Anita Rao (`STF-0002`) on duty.

`TestIssueFormRendersWalkthroughLabels` — `GET /issue/new?productId=PRD-0001` contains
`Someone is taking`, `Thursday, 3 September · 2:18 pm`, `Product`, `How many`,
`890 on hand right now`, `Most you can issue is 890.`, `Who is taking it`,
`Department`, `Their mobile`, `Person incharge (giving it)`,
`Anita Rao <span class="sm">(you)</span>`, `Your mobile`, `99001 34562`, `Cancel`.

`TestIssuePrefillsKnownTaker` — `GET /api/people?q=Ravi Menon` (or the server-rendered
re-post) yields `Catering` and `98861 40023`, and the re-rendered form contains
`This person has taken things before. Their details are filled in.`

`TestPersonPickerFindsByNameOrMobile` — `GET /api/people?q=Ravi`,
`?q=98861`, `?q=9886140023` and `?q=98861 40023` each return Ravi Menon, and the
rendered suggestion row is `Ravi Menon · 98861 40023 · Catering`.

`TestPersonPickerShowsTwoPeopleWithOneName` — over a T1 copy holding issues to
`Ravi Kumar / 98861 40023 / Catering` and `Ravi Kumar / 97740 11298 / Security`,
`?q=Ravi Kumar` returns both, in that order, and the rendered list is exactly
`Ravi Kumar · 98861 40023 · Catering`, `Ravi Kumar · 97740 11298 · Security`,
`+ New person named Ravi Kumar`.

`TestPersonPickerAlwaysOffersANewPerson` — the `+ New person named <typed>` row is
present when there are no matches, one match, and many matches; and is absent from
`GET /api/people?scope=log&q=<the same text>` in each of those three cases.

`TestPersonPickerNeverBlocks` — post an issue of 10 chairs to `Ravii Varma` /
`98861 40023` (a misspelling of an existing taker, same mobile as Ravi Menon). It saves
with no confirmation step, returns 303, and the response never contains `Did you mean`,
`already`, `duplicate` or `Are you sure`. `PeopleHolding` afterwards shows Ravi Menon
and Ravii Varma as two rows, which is accepted.

`TestIssueFillsMobileForASingleMatch` — post with `takerName=Ravi Menon` and an empty
`takerMobile`; the stored issue has `TakerMobile: "98861 40023"` and
`TakerDepartment: "Catering"`, taken from his last issue, so he does not become a
second, mobile-less Ravi Menon.

`TestIssueDoesNotGuessBetweenTwoMatches` — with both Ravi Kumars present, posting
`takerName=Ravi Kumar` and an empty mobile stores an empty mobile. The software fills a
blank only when there is exactly one candidate; it never picks one of two.

`TestIssueAmberBannerForRavi` — the form rendered with `takerName=Ravi Menon` and
`takerMobile=98861 40023` contains `banner warn` and the sentence
`Ravi Menon is already holding 40 chairs and 2 round tables from earlier today.`

`TestIssueAmberBannerIsPerMobile` — with both Ravi Kumars holding stock, the form
rendered for `Ravi Kumar / 98861 40023` names only that man's holdings and never sums
the two.

`TestIssueAmberBannerDropsTodayClauseForOlderHoldings` — rendered with
`takerName=Joseph D'Cruz` (issues dated 1 September), the banner reads
`Joseph D'Cruz is already holding 120 chairs and 25 extension boards.` with no
`from earlier today`.

`TestIssueNoBannerForNewPerson` — `takerName=Meera Pillai` renders no `banner warn`.

`TestIssue10ChairsToRavi` — `POST /issue/new` with
`productId=PRD-0001&quantity=10&takerName=Ravi Menon&takerDepartment=Catering&takerMobile=98861 40023&issuedAt=2026-09-03T14:18`:
- 303 to `/stock?saved=ISS-0008`;
- the stored issue matches ISS-0008 in `01-data-model.spec.md` exactly, including
  `PersonInchargeName: "Anita Rao"` and `PersonInchargeMobile: "99001 34562"`;
- `OnHand(PRD-0001) == 880`;
- the redirect target contains
  `Gave 10 chairs to Ravi Menon. Chairs: 880 on hand.`

`TestIssueRefuses900From890` — `quantity=900` returns 200 containing
`You have 890 chairs. You cannot give out more than 890.`; the register still has 7 issues and
`OnHand` is still 890. This is the walkthrough's "Nobody can issue 900 chairs out of
890 by mistyping."

`TestIssueAllowsExactly890` — `quantity=890` succeeds and leaves `OnHand == 0`; a
following post of 1 chair is refused with
`There are no chairs left.`

`TestIssueZeroStockSentence` — a direct post for `PRD-0004` (Extension boards, 0 on
hand) is refused with `There are no extension boards left.` and
never with a sentence containing `give out more than 0`; at zero the refusal is
`There are no chairs left.`

`TestIssueRefusesZeroAndGarbage` — `quantity=0`, `-3`, `abc`, `10.5` each return 200
with `Type how many chairs they are taking.` and write nothing.

`TestIssueRefusesEmptyTaker` — `takerName=   ` returns 200 with
`Who is taking it? Type their name.`

`TestIssueRefusesProductWithNoStock` — `productId=PRD-0004` (Extension boards, 0 on
hand) is refused and the product does not appear in the `mode=instock` picker at all.

`TestIssueIgnoresPersonInchargeFromForm` — a post that also carries
`personInchargeName=Somebody Else&personInchargeMobile=00000 00000` stores
`Anita Rao` / `99001 34562`.

`TestIssueEditableTimestampIsHonoured` — `issuedAt=2026-09-03T13:05` stores
`2026-09-03T13:05:00+05:30` in `IssuedAt` while `RecordedAt` remains
`2026-09-03T14:18:00+05:30`.

`TestIssueRevalidatesAtSaveTime` — render the form against 890 on hand, then mutate the
store so only 5 chairs remain, then post `quantity=10`. The response is 200 with
`You have 5 chairs. You cannot give out more than 5.` and nothing is written.

`TestIssueKeepsTypedValuesOnRefusal` — a refused post retains `Ravi Menon`, `Catering`
and `98861 40023` in the re-rendered inputs.

`TestIssueButtonLabel` — the form with 10 chairs and Ravi Menon filled in contains
`Issue 10 chairs to Ravi`.

## Acceptance criteria

1. `go test ./internal/web/ -run 'TestIssue|TestPersonPicker' -count=1` passes with all
   twenty-four tests.
2. The success path returns 303 with `Location: /stock?saved=ISS-0008`; every refusal
   returns 200.
3. `grep -n 'PersonInchargeName' internal/web/issue.go` shows it assigned only from the
   on-duty record, and `grep -n 'FormValue("personIncharge' internal/web/issue.go`
   returns nothing.
4. `grep -n 'CheckIssue' internal/web/issue.go` shows a call inside the
   `store.Update` closure, not only before it.
5. No test in the package leaves `OnHand` negative:
   `TestOnHandNeverNegativeAcrossFixture` from spec 03 is re-run over the register
   produced by every issue test.
6. The person picker never nags:
   `grep -rniE 'did you mean|are you sure|already exists|duplicate' internal/web/templates/issue.html internal/web/templates/person-picker.html`
   returns nothing. (The same grep over `products.go` and the product picker **must**
   match — the asymmetry is deliberate.)

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestIssue' -count=1 -v
go vet ./internal/web/
```

## Open

1. **The over-issue sentence.** The walkthrough gives the hint `Most you can issue is
   890.` but never the refusal itself. Two sentences are specified above —
   `You have 890 chairs. You cannot give out more than 890.` and, at zero,
   `There are no chairs left.` Both need plain-language review;
   the required tests assert them as written, so a change of wording changes two tests
   and nothing else.
2. **The amber banner says "2 tables"** in the mockup where the product is named
   `Round tables`. This spec renders the product's real name (`2 round tables`) rather
   than an abbreviation the software cannot derive. Confirm.
3. **The `Time taken` field label.** The mapping table requires an editable outward
   date and time; the mockup shows no such input. Label chosen here — confirm.
4. **Whether the `+ New person named …` row should also appear when the typed text is a
   mobile number.** `+ New person named 98861 40023` reads oddly. Recommend: when what
   was typed is all digits, the row is hidden and the name field is left for the desk to
   fill; a person is never created from a bare number.
5. **One product per issue.** Ravi took 40 chairs and 2 round tables at 09:40 as two
   separate issue records. The walkthrough shows them as two lines and shows no
   multi-product form. Confirmed by the mockup, noted here so nobody adds a basket.
