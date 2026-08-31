# Spec: The Reading Screens — Stock, Out With People, Stuff Came In, Suppliers

## Objective

The screen that replaces the shift handover. The person walking in at 2pm reads it and
knows what the person who left at 1pm knew: what is on hand, and what is out with whom.
Nothing on these screens changes any record — the three buttons on Stock are doors into
the three entry flows, and the Suppliers tab has no button at all.

## Context

- Owns `internal/web/views.go` and `internal/web/templates/{stock,out,inwards,suppliers}.html`.
- Depends on `03-stock-arithmetic.spec.md` for every number,
  `04-server-and-shell.spec.md` for the tab bar, `08-issue.spec.md` for the person
  partial used by the `Came back` section and the `/return/new?q=` links, and
  `11-corrections.spec.md` for the `Fix this` links and the tombstone rendering.
- All four are `GET`-only. No form on any of them; the only controls are links into the
  three entry flows and into the correction screens.
- **No redirect anywhere in the program targets `/suppliers`.** Saves land on `/stock`
  (the three entry flows) or on `/inwards` and `/out` (corrections), so no confirmation
  sentence — `Took back 45 chairs` among them — can ever appear on the one page asserted
  to carry no debt vocabulary.

## Contract

### `GET /stock` — the Stock tab

Four tiles across the top, in this order, from `TileCounts(reg, now)`:

| Label | Value | Note |
|---|---|---|
| `Products` | `Products` | |
| `Out right now` | `OutRightNow` | |
| `People holding` | `PeopleHolding` | |
| `Out over 2 days` | `OutOverTwoDays` | rendered amber (`color:var(--warn)`) when above 0 |

Then the stock table, from `StockRows(reg)`, columns
`Product`, `Type`, `Came in`, `Out`, `On hand`, and an unlabelled action column:

- `Type` is a pill: `Rent` (`pill rent`) or `Purchase` (`pill sale`).
- The three numeric columns are right-aligned, `On hand` in `<strong>`.
- `On hand` of 0 is rendered in red (`color:var(--bad)`).
- Action column: a small `Issue` button linking to `/issue/new?productId=<id>` when
  `OnHand > 0`; a disabled-looking `None left` chip (`opt`) when it is 0.

Below the table, three buttons in this order: primary `+ Stuff came in` →
`/inward/new`; ghost `Someone is taking` → `/issue/new`; ghost `Someone is returning`
→ `/return/new`.

Banners: the `?saved=<id>` confirmations specified by 07, 08 and 09, plus the recovery
warning from 04.

### `GET /out` — the Out with people tab

Not mocked in the walkthrough; shape recommended here (see Open). One block per person
from `PeopleHolding(reg)`, alphabetical — and a person is a name **and** a mobile, so
`Ravi Kumar / 90011 22334` and `Ravi Kumar / 93400 55118` get a block each:

```
Ravi Menon · Catering · 98861 40023 — holding 42
  Chairs         Issued 9:40 am by Suresh Kumar · 40 taken, 0 back      40 out
  Round tables   Issued 9:40 am by Suresh Kumar · 2 taken, 0 back        2 out
```

using the same `outrow` markup and the same middle-line wording as the returning
screen, so the two read identically. A line whose `IssuedAt` is more than 48 hours
before `now` carries an amber `pill` reading `Out over 2 days`. Each person block has a
ghost button `Someone is returning` linking to
`/return/new?q=<their mobile, or name when no mobile>`.

Where a return recorded a shortfall against a line, the remark is shown beneath it in
`sm` type: `5 chairs short — 5 chairs broke during setup near the stage. Ravi informed.`
prefixed with `Won't come back:` or `Still expected back:` according to the
disposition.

Each issue line carries `id="<issueId>"` and a `Fix this` link to
`/entry/<issueId>/edit` (`11-corrections.spec.md`) and, beneath it, any correction
already made, in `sm` type:
`Changed how many from 40 to 45 by Anita Rao, 3:20 pm`. Each `Came back` line carries
`id="<returnId>"`. The anchors let `12-activity-log.spec.md` link to a line as
`/out#ISS-0008` or `/out#RET-0001`; nothing else depends on them.

When nobody is holding anything: `Nothing is out with anybody right now.`

**Below the person blocks, a section headed `Came back`** — every live return, newest
`ReturnedAt` first, whatever became of the issues behind it:

```
45 chairs from Ravi Menon · 6:05 pm · taken back by Imran Sheikh    Fix this
   Won't come back: 5 chairs broke during setup near the stage. Ravi informed.
```

This section is a flat list, **not** nested under the outstanding lines. A return whose
issues are now fully settled has no outstanding line to hang from, and that is exactly
the return somebody needs to reach when they typed 45 and meant 4. When there are none:
`Nothing has come back yet.`

### `GET /inwards` — the Stuff came in tab

Not mocked; shape recommended here (see Open). One table, newest `RecordedAt` first,
columns `Date received`, `Product`, `How many`, `Type`, `Came from`, `Challan no.`,
`Received by`, and a final unlabelled column holding a `Fix this` link to
`/entry/<inwardId>/edit`. **Each row carries `id="<inwardId>"`**, so
`12-activity-log.spec.md` can link straight to it as `/inwards#INW-0007`. `Came from` renders `nobody wrote it down` in `sm` type when
blank. When there are no inwards: `Nothing has come in yet.`

Corrections show under the row they belong to, in `sm` type, oldest first:
`Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am`.

**Deleted inwards stay in this list**, greyed and struck through, with their reason
beneath: `Deleted by Suresh Kumar, 10:47 am — Entered twice by mistake.` They count
towards no figure anywhere; they are here so that a number which changed can be
accounted for by whoever asks. They carry no `Fix this` link.

### `GET /suppliers` — the Suppliers tab

**This page keeps no score.** The store's job ends when the stock is back in the store;
somebody else gets the rented goods back to the people who own them. So there is no
`Given back` column, no `Still owed` column, no settling and no money. The page answers
one question: what came in, and who sent it.

Read-only. No button anywhere on it.

Then the table from `SupplierRows(reg)`, columns `Supplier`, `Product`, `Came in`,
`Type`:

- `Came in` is the total that supplier sent of that product, right-aligned, in
  `<strong>`.
- `Type` is the same pill as the stock table: `Rent` (`pill rent`) or `Purchase`
  (`pill sale`).
- When `WontComeBack > 0`, a `sm` amber line under the `Came in` figure:
  `<n> broken or lost`. It is a note about the goods, not a debt — of the 890 chairs
  Sharma Tent House sent, 5 are gone.
- Non-rent rows: the `Supplier` cell reads `we bought it` in `sm` type when no supplier
  was recorded, and never carries the broken-or-lost note.

Above the table, a plain line whenever anything is out with people — **not** a
`banner bad`. Something is out with somebody for the whole of a live gathering, so a red
alarm here would be permanently on and would teach the reader to skip red banners,
including the recovery one that matters:
`<OutRightNow> things are still out with people.`
Omitted when `OutRightNow` is 0.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/views.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/stock.html`
- `/home/asim/Projects/inventory-management/internal/web/templates/out.html`
- `/home/asim/Projects/inventory-management/internal/web/templates/inwards.html`
- `/home/asim/Projects/inventory-management/internal/web/templates/suppliers.html`
- `/home/asim/Projects/inventory-management/internal/web/views_test.go`

## Required tests

Clock `2026-09-03T10:00:00+05:30` over `WalkthroughT0()` unless stated.

`TestStockTilesAtT0` — `GET /stock` renders the four tiles with values `5`, `352`, `4`
and `3`, each next to its exact label. (The walkthrough's mockup shows 14 / 327 / 18 / 3
from a fuller register than the fixture; the definitions in
`03-stock-arithmetic.spec.md` are what is normative. See Open.)

`TestStockTableAtT0` — the table has header cells `Product`, `Type`, `Came in`, `Out`,
`On hand`, and five rows in the order Chairs, Charcoal sacks, Extension boards, Round
tables, Water drums (20L), with figures 700/310/390, 12/0/12, 25/25/0, 60/12/48 and
40/5/35.

`TestStockShowsZeroInRedWithNoIssueButton` — the Extension boards row contains
`var(--bad)` and `None left`, and contains no link to
`/issue/new?productId=PRD-0004`.

`TestStockIssueButtonsLinkToProduct` — the Chairs row contains
`/issue/new?productId=PRD-0001`.

`TestStockPills` — Chairs and Round tables and Extension boards carry `pill rent` with
the text `Rent`; Water drums (20L) and Charcoal sacks carry `pill sale` with `Purchase`.

`TestStockThreeButtons` — the body contains `+ Stuff came in`, `Someone is taking` and
`Someone is returning`, linking to `/inward/new`, `/issue/new` and `/return/new`.

`TestStockSavedBanner` — `GET /stock?saved=INW-0007` over the T1 register contains
`Added 500 chairs. Chairs: 890 on hand.`

`TestStockAfterFullWalkthroughSequence` — over the T3 register, the Chairs row reads
`1200`, `275`, `925` and the `Out right now` tile reads `317`.

`TestOutWithPeopleAtT0` — `GET /out` lists four people in the order Farida Begum,
Joseph D'Cruz, Lakshmi Iyer, Ravi Menon; Ravi's block contains
`Ravi Menon · Catering · 98861 40023`, `holding 42`, and both his lines with the exact
middle-line wording `Issued 9:40 am by Suresh Kumar · 40 taken, 0 back`.

`TestOutWithPeopleFlagsAgedLines` — ISS-0001, ISS-0002 and ISS-0007 each carry the
`Out over 2 days` pill; ISS-0003 and ISS-0005 do not.

`TestOutWithPeopleShowsShortfallRemark` — over the T3 register, Ravi's chairs line shows
`5 out` and beneath it
`Won't come back: 5 chairs broke during setup near the stage. Ravi informed.`

`TestOutWithPeopleEmpty` — over a register with no outstanding issues, the body contains
`Nothing is out with anybody right now.`

`TestInwardsTabListsNewestFirst` — over the T1 register, the first data row is
INW-0007 (`2026-09-03`, Chairs, 500, Rent, `Sharma Tent House`, `STH/4471`,
`Suresh Kumar`) and INW-0002 shows `nobody wrote it down` in its `Came from` cell.

`TestInwardsTabLinksToFixThis` — every live row carries a link to
`/entry/<its id>/edit`, so INW-0007's row contains `/entry/INW-0007/edit`.

`TestRowsCarryAnchors` — over T3, `GET /inwards` contains `id="INW-0007"` on the
INW-0007 row and an `id` for every other inward including the deleted ones;
`GET /out` contains `id="ISS-0008"` and `id="RET-0001"`.

`TestInwardsTabShowsCorrectionsAndTombstones` — over a T1 copy where INW-0007 was
changed from 500 to 50 by Suresh Kumar at 10:45 and INW-0002 was deleted by him at
10:47 with the reason `Entered twice by mistake.`, the page contains
`Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am` and
`Deleted by Suresh Kumar, 10:47 am — Entered twice by mistake.`, the deleted row has no
`/entry/INW-0002/edit` link, and the Chairs stock row on `/stock` reads
`Came in 440`.

`TestOutTabListsReturnsFlat` — over the T3 register, `/out` contains a `Came back`
heading and the line `45 chairs from Ravi Menon · 6:05 pm · taken back by Imran Sheikh`
with a link to `/entry/RET-0001/edit`. After a further return settling Ravi's last 5
chairs, both returns are still listed even though ISS-0003 and ISS-0008 no longer appear
in the outstanding blocks.

`TestOutTabSeparatesTwoPeopleWithOneName` — over a T2 copy with 6 chairs issued to
`Ravi Kumar / 90011 22334` and 4 to `Ravi Kumar / 93400 55118`, `/out` shows two blocks,
one reading `holding 6` and one `holding 4`, each showing its own mobile. No block reads
`holding 10`.

`TestSuppliersBannerCountsOut` — over T0, `GET /suppliers` contains
`352 things are still out with people.`

`TestSuppliersBannerHiddenWhenNothingOut` — over a register with nothing out, the
sentence does not appear.

`TestSuppliersTableAtT3` — over the T3 register the page shows, in order:
`Gupta Electricals` / `Extension boards` / `25` / `Rent`;
`Sharma Tent House` / `Chairs` / `890` / `Rent`, with `5 broken or lost` beneath the
890;
`Sharma Tent House` / `Round tables` / `60` / `Rent`;
then three `we bought it` rows for Chairs (310), Charcoal sacks (12) and Water drums
(20L) (40), each showing the `Purchase` pill and no note.

`TestSuppliersHasNoDebtColumns` — the `/suppliers` body contains the header cells
`Supplier`, `Product`, `Came in` and `Type`, and does **not** contain `Given back`,
`Still owed`, `All back`, `owed`, `Took` or `Balance` in any casing.

`TestSuppliersHasNoButtons` — the `/suppliers` body contains no `<form`, no
`<button`, no `class="btn`, and none of the words `settle`, `paid`, `amount`, `₹` or
`Rs`.

## Acceptance criteria

1. `go test ./internal/web/ -run 'TestStock|TestOut|TestInwards|TestSuppliers' -count=1` passes with all twenty-two tests.
2. `grep -rniE 'settle|amount|invoice|payment|₹|rupee|still owed|given back' internal/web/templates/` returns nothing.
3. `grep -n 'method="post"\|<button' internal/web/templates/suppliers.html` returns nothing.
4. All four routes return 200 over `WalkthroughT0()` and over an empty register — a
   test loops the four paths against both and asserts the status and a non-empty body.
5. `grep -rn '"/suppliers' --include=*.go internal/web/ | grep -i 'redirect\|Location'`
   returns nothing.
6. No template calls a computation of its own:
   `grep -nE '\{\{ *[0-9]|add |sub ' internal/web/templates/*.html` returns nothing.
   Every number comes from `internal/register`.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -run 'TestStock|TestOut|TestInwards|TestSuppliers' -count=1 -v
grep -rniE 'settle|amount|invoice|payment' internal/web/templates/   # must print nothing
```

## Open

1. **The "Out with people" and "Stuff came in" tabs are named but never drawn.** The
   layouts above are recommended, built from data the register already holds and reusing
   the `outrow` markup from the returning screen. Confirm both, or sketch them.
2. **The mockup's tile figures (14 / 327 / 18) come from a larger register than the
   fixture** and 327 happens to equal the sum of only the first three stock rows. The
   definitions are treated as normative and the fixture asserts 5 / 352 / 4 / 3.
   Confirm that `Out right now` includes every product, extension boards included.
3. **Supplier row ordering.** Alphabetical by supplier, so Gupta Electricals comes
   before Sharma Tent House; the mockup shows Sharma first. Confirm alphabetical, or
   name the intended order (first-seen? largest first?).
4. **Where returns are listed.** The `Came back` section at the foot of the
   "Out with people" tab is this spec's choice: returns had no home in the four
   approved tabs, and one is needed so a mistyped return can be reached. Confirm the
   placement, or name a better one.
5. **Whether `/out` should offer a "Someone is returning" shortcut per person.**
   Recommended above; not in the walkthrough.
