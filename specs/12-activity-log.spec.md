# Spec: Who Did What

## Objective

Somebody notices a number looks wrong. One screen answers the question that always
follows: who did this, and when. It is one chronological list of everything that has
ever happened at the desk — stock in, stock out, stock back, entries fixed, entries
deleted, products added, shifts started — newest first, in the same plain words the rest
of the register uses, filtered down to a day and a person in two taps.

**This is a view, not new data.** Every record already carries who and when. Nothing on
this page is stored, nothing on this page is written, and there is no log table anywhere
in the file. The list is walked out of the records themselves, so it can never drift
from what it describes.

## Context

- Owns `internal/register/log.go`, `internal/register/log_test.go`,
  `internal/web/log.go`, `internal/web/templates/log.html`,
  `internal/web/log_test.go`.
- Depends on `01-data-model.spec.md` (`Inward`, `Issue`, `Return`, `Change`, `Deletion`,
  `Product`, `Staff`, `Register.ShiftStartedAt`), `03-stock-arithmetic.spec.md`
  (`PersonID`, `PersonOf`, `PersonSummary`, `FoldKey`, `MobileKey`),
  `04-server-and-shell.spec.md` (shell, tabs, the shift guard, `clock`),
  `06-products.spec.md` (the product picker), `08-issue.spec.md` (the person picker and
  `/api/people`), `10-views.spec.md` (the tabs this page links into),
  `11-corrections.spec.md` (the correction and deletion sentences).
- Route: `GET /log`. No other method and no other route belongs to this spec.

### The one function in `internal/register` that reads tombstones

`03-stock-arithmetic.spec.md` is absolute: every function in `arith.go` skips deleted
records, and its acceptance criterion 6 greps `arith.go` for direct iteration of
`r.Inwards`, `r.Issues` and `r.Returns`. The log is the single place where a deleted
record must still be seen, so its builder lives in a **new file**,
`internal/register/log.go`, which iterates the raw slices directly. Spec 03's criterion
6 names `arith.go` and stays true unchanged. `log.go` takes no clock and does no I/O,
exactly like `arith.go`.

### Reaching a record: link, do not edit

Each row links to the record **on the tab where it already lives** —
`/inwards#INW-0007`, `/out#ISS-0008`, `/out#RET-0001` — and never to
`/entry/{id}/edit`. Two reasons: `11-corrections.spec.md` defines the edit screen's
return path as the tab the `Fix this` link came from, carried in a `from` field, and
introducing a third origin would change that contract; and the log's job is to show
someone the record in its context, from where the existing `Fix this` link is one tap
away. This keeps "no control on this page changes anything" true in the strongest
possible sense.

This requires `id="<recordID>"` on the inward rows in `inwards.html` and on the issue
lines and `Came back` lines in `out.html`. Listed under Files to modify.

## Contract

### `LogEntry` — `internal/register/log.go`

```go
type LogKind string

const (
    LogProductAdded LogKind = "product_added"
    LogPersonAdded  LogKind = "person_added"
    LogCameIn       LogKind = "came_in"
    LogWentOut      LogKind = "went_out"
    LogCameBack     LogKind = "came_back"
    LogCorrected    LogKind = "corrected"
    LogDeleted      LogKind = "deleted"
)

type LogEntry struct {
    At          time.Time  // when it was recorded — the field the list sorts on
    Kind        LogKind
    RecordID    string     // "INW-0007", "ISS-0008", "RET-0001", "PRD-0003", "STF-0002"
    RecordTab   string     // "/inwards" or "/out"; "" when the entry has no record page
    RecordDeleted bool     // the record this row is about carries a tombstone

    Who         string     // the actor; "" only when CreatedBy was empty — see below
    WhoMobile   string     // the actor's mobile when known; "" otherwise

    PersonName   string    // the subject person; "" when the event has none
    PersonMobile string
    PersonDepartment string

    ProductID   string     // "" only for LogPersonAdded
    ProductName string
    Quantity    int        // 0 for LogProductAdded, LogPersonAdded, LogCorrected, LogDeleted

    Supplier    string     // LogCameIn only; "" when none was recorded
    ReceivedBy  string     // LogCameIn only; the name on the record

    HappenedAt  time.Time  // LogWentOut: IssuedAt. LogCameBack: ReturnedAt. Otherwise zero.
    ReceivedOn  string     // LogCameIn only, "2026-09-03"

    ShortQuantity    int         // LogCameBack only
    ShortDisposition Disposition // LogCameBack only
    Remark           string      // LogCameBack only

    Change      *Change    // set only when Kind == LogCorrected
    ChangeIndex int        // index within the record's Changes slice; 0 otherwise
    Deletion    *Deletion  // set only when Kind == LogDeleted
}

func LogEntries(r *Register) []LogEntry
```

`LogEntries` walks the raw slices — **including deleted records** — and emits:

| Source | Kind | `At` | `Who` | Subject person |
|---|---|---|---|---|
| each `Product` | `product_added` | `CreatedAt` | `CreatedBy` | none |
| each `Staff` | `person_added` | `CreatedAt` | `CreatedBy` | the person added — `Name` / `Mobile` |
| each `Inward` | `came_in` | `RecordedAt` | `RecordedBy` | none — a supplier is not a person |
| each `Issue` | `went_out` | `RecordedAt` | `PersonInchargeName` | `TakerName` / `TakerMobile` / `TakerDepartment` |
| each `Return` | `came_back` | `RecordedAt` | `TakenBackBy` | `ReturnerName` / `ReturnerMobile` |
| each `Change` on any record | `corrected` | `Change.At` | `Change.By` | that record's subject person |
| each `Deletion` on any record | `deleted` | `Deletion.At` | `Deletion.By` | that record's subject person |

`RecordDeleted` is true on **every** row about a record that carries a tombstone,
including the original `came_in` / `went_out` / `came_back` row. Deleted records are
never hidden from this list; the log is the one place nothing is ever hidden.

`ProductName` is resolved from `Register.Products`; when a `ProductID` matches no
product, `ProductName` is the empty string and the row still appears.

`Who` is empty in exactly one situation: the **first** staff member on a fresh
register, added before anybody is on duty (`05-shift-and-people.spec.md`). There is
genuinely no one to name, so the row renders with no who rather than a placeholder.

### There are no shift lines

The log carries no "a shift started" event, and this spec does **not** add a `Shifts`
slice to the register. Every entry already carries the person who made it, so the log
answers "who did this" without a shift record. Showing only the *current* shift start —
which is all `ShiftStartedAt` could ever give, since `POST /shift/start` overwrites it —
would be worse than showing none: a log that displays one instance of a category it
cannot display historically teaches the reader it is complete when it is not. If
somebody later asks who was at the desk between 2 and 6 despite entering nothing, the
slice gets added then.

### Ordering

Newest first, on `At` — **the time the thing was recorded, not the time it is said to
have happened.** A backdated issue typed at 6pm belongs at 6pm, because the log
reconstructs what someone did, not what the stock did.

The comparison, in order, until one differs:

1. `At`, **descending**.
2. `rank(Kind)`, **descending**, where
   `product_added=1, person_added=2, came_in=3, went_out=4, came_back=5, corrected=6, deleted=7`.
3. `RecordID`, **descending** (string comparison; IDs are fixed-width per prefix).
4. `ChangeIndex`, **ascending**.

Rule 4 is a deliberate exception to newest-first and must not be reversed. A single
edit that changes two fields appends two `Change` entries at one instant, in field
order (`11-corrections.spec.md`); they are one action, and the reader must see them in
the order the form is laid out, top to bottom.

`LogEntries` is deterministic: the same register always produces the same slice, in the
same order, with no dependence on map iteration.

### Filtering

```go
type LogFilter struct {
    Day       string  // "2026-09-03", or "" meaning every day
    Kinds     []LogKind // empty means every kind
    ProductID string  // "" means every product
    Query     string  // "" means everybody
}

func FilterLog(r *Register, entries []LogEntry, f LogFilter, loc *time.Location) []LogEntry
```

`loc` is the location `Day` is interpreted in — the server's local zone. No clock is
read inside the function.

**Filters combine. An entry is shown only if it matches every active filter, and an
entry that has no value for a filtered dimension does not match.** That one sentence
settles every awkward pairing: a person-added line disappears under a product filter, a
product-added line disappears under a person filter unless the query names whoever added
it, and nobody has to guess.

- **Day** — `At.In(loc)` falls on that calendar date.
- **Kinds** — `Kind` is in the set.
- **Product** — `ProductID` equals the filter. A `product_added` row matches the filter
  for the product it added. A `person_added` row never matches.
- **Query** — matches when, after `strings.TrimSpace`, any of these holds:
  - `FoldKey(Query)` is a substring of `FoldKey(Who)`, `FoldKey(PersonName)`,
    `FoldKey(PersonDepartment)` or `FoldKey(ReceivedBy)`;
  - `MobileKey(Query)` is non-empty and is a substring of `MobileKey(WhoMobile)` or
    `MobileKey(PersonMobile)`.

  So `98861` finds every row about Ravi Menon, `Imran` finds everything Imran Sheikh
  did **and** everything he took back, and `cater` finds Ravi's issues.

### The person picker on this page

The picker in `08-issue.spec.md` is backed by `FindPeople`, which searches only people
with `TotalOut > 0` and never sees staff at all
(`03-stock-arithmetic.spec.md`). That is the wrong population for this page: the person
being looked for is usually a staff member, and is often somebody who has already
returned everything.

Two additions, both minimal:

```go
// internal/register/log.go
func PeopleInLog(r *Register) []PersonSummary
func FindPeopleInLog(r *Register, query string) []PersonSummary
```

`PeopleInLog` is the whole cast of the log, deduplicated by `PersonID`:

- every `Staff` entry — `Name`, `Mobile`, `Department: ""`;
- every distinct `PersonOf(TakerName, TakerMobile)` across **all** issues, deleted
  included;
- every distinct `PersonOf(ReturnerName, ReturnerMobile)` across **all** returns,
  deleted included.

Where one `PersonID` appears more than once, the display `Name`, `Mobile` and
`Department` come from the most recent record that carries them, falling back to the
`Staff` entry. `TotalOut` is always 0 and `Lines` is always nil — this page keeps no
score. Sorted by `Name` A→Z, ties by `Mobile`.

`FindPeopleInLog` applies the same three matching rules `FindPeople` uses — name
substring, `MobileKey` substring, department substring — over that population. An empty
query returns everything.

Served as `GET /api/people?scope=log&q=<text>`, in the same JSON shape and rendered
through the same `person-picker.html` partial and the same `person-picker.js`, so the
person at the desk meets one picker in this program and not two.

**One deviation from `08-issue.spec.md`, which says the `+ New person named <typed>`
row is "always last and always present": in `scope=log` that row is not rendered.**
Nobody is created from a read-only page, and offering it would be a control that
appears to change something. Spec 08 needs the one-line amendment "always present
except in `scope=log`"; listed under Follow-ups.

### `GET /log` — the Who did what tab

Chrome title `Who did what`. Guarded by the shift guard in
`04-server-and-shell.spec.md` like every other tab: 303 to `/shift` when nobody is on
duty.

Query parameters, all optional:

| Parameter | Values | Default |
|---|---|---|
| `day` | `YYYY-MM-DD`, or `all` | the calendar date of `now` in the server's zone |
| `kind` | `all`, `came_in`, `went_out`, `came_back`, `corrected`, `deleted` | `all` |
| `productId` | a product ID | unset — every product |
| `q` | free text, from the person picker | unset — everybody |

An unparseable `day`, an unknown `kind` and an unknown `productId` each fall back to
that parameter's default. The page renders normally, with no banner and no error: a bad
value can only arrive from a hand-typed address, and there is nothing here to break.

`kind` deliberately offers only the five the user named. **`product_added` and
`person_added` rows appear only under `kind=all`** — they are not things anybody
filters for, and giving them buttons would clutter the row of five that matters.

#### The filter bar

Above the list, in this order:

1. **`Which day`** — a `<input type="date" name="day">` pre-filled with the active day,
   inside a `GET` form together with the person field; and beside it an `opt` link
   `Every day` → `/log?day=all` carrying the other active filters. When `day=all` is
   active, `Every day` carries the `on` class and a second `opt` link reads `Today`.
2. **`What happened`** — six `opt` links, never a form:
   `Everything`, `Came in`, `Went out`, `Came back`, `Entry fixed`, `Entry deleted`. The
   active one carries `on`. Each link keeps every other active filter.

   The last two are named `Entry ...`, not `Corrected` and `Deleted`, because the first
   three describe things that happened to **stock** and the last two describe things
   that happened to **entries** — the one distinction this whole page exists to make,
   which a flat list of five blurs. `fixed` is also the desk's own word, the one the
   `Fix this` links already use; `corrected` is record-keeping vocabulary.
3. **`Which person`** — the person picker above, `scope=log`, a text input named `q`
   inside the same `GET` form. Beside it, when a query is active, an `opt` link
   `Anybody` that clears it.
4. **`Which product`** — the product picker from `06-products.spec.md`, `mode=all`,
   **without** its add-a-new-product row, writing `productId` into the same `GET` form.
   Beside it, when a product is active, an `opt` link `Any product` that clears it.

The form carries one primary `btn` reading `Show`. Every other control on the page is a
link. There is no free-text search box over remarks, suppliers or challan numbers: the
four filters above answer the question the page exists for, and the register is small
enough that a day plus a person is always enough to find a row by eye.

#### The list

Rows are grouped under a day heading, newest day first:
`Thursday, 3 September`. Grouping applies whether one day or every day is shown, so the
row itself never has to carry a date.

Four columns: `Time`, `Who did it`, `What happened`, and a final unlabelled column.

The first column heading is `Who did it`, not `Who`. Every row carries two people — the
person who made the entry and the person the stock moved to or from — and telling those
two apart is the entire purpose of this page.

- **`Time`** — `clock(At)`, so `6:05 pm`.
- **`Who did it`** — the actor's full name. **Never a first name**, on any row: the amber
  banner and the issue button abbreviate to `Ravi` elsewhere, and this page must not,
  because the whole point of it is telling two people apart. When it is empty,
  the cell reads `nobody was on duty yet` in `sm` type. Not `/inwards`' sentence for a
  missing supplier: the entry plainly *was* written down, and the only case where the
  actor is unknown is the very first person added to a fresh register, before anybody
  had started a shift.
- **`What happened`** — a main line, and up to two `sm` lines beneath it.
- **The last column** — one link, `Go to this entry`, to `RecordTab + "#" + RecordID`.
  Empty for `product_added` and `person_added`, which have no page to go to.

Main lines, by kind. The noun always travels with the number and no label inflects:

| Kind | Main line |
|---|---|
| `came_in` | `500 chairs came in from Sharma Tent House` — or `500 chairs, no supplier written down` when none was recorded |
| `went_out` | `10 chairs went out to Ravi Menon` |
| `came_back` | `45 chairs came back from Ravi Menon` |
| `corrected` | `Fixed this entry: ` + the record in words (below), then the change on the line beneath |
| `deleted` | `Deleted this entry: ` + the record in words (below), struck through |
| `product_added` | `Chairs added to the product list.` |
| `person_added` | `Anita Rao added to the people list.` |

**The record in words** is one shared helper,
`entryName(reg *register.Register, recordID string) string`, living in
`internal/web/corrections.go` beside `changePhrase` and registered as a template
function. It gives
the record's current contents in the same shape `11-corrections.spec.md` uses for its
sub-headings, minus the trailing time clause the log's own `Time` column already
supplies:

- inward — `500 chairs from Sharma Tent House`, or `500 chairs, no supplier written down`
  when none was recorded. This matches the `came_in` main line exactly; the two must not
  drift, or a deleted inward and its own deletion row sit adjacent describing the same
  event in two different phrasings;
- issue — `10 chairs to Ravi Menon`;
- return — `45 chairs back from Ravi Menon`.

The `sm` lines beneath, in this order when each applies:

| Condition | `sm` line |
|---|---|
| `Kind == corrected` | `changePhrase(*Change)` — see below |
| `Kind == deleted` | `Deleted — Entered twice by mistake.` (`"Deleted — " + Deletion.Reason`) |
| `Kind == went_out` and `HappenedAt != At` | `Taken at 1:05 pm, typed in at 2:18 pm.` — both clocks are named, because the Time column already shows one of them and nothing would say which is which |
| `Kind == came_back` and `HappenedAt != At` | `Came back at 5:30 pm, typed in at 6:05 pm.` |
| `Kind == came_in` and `ReceivedOn != At.In(loc).Format("2006-01-02")` | `Received on 4 September.` |
| `Kind == came_in` and `ReceivedBy != Who` and `ReceivedBy != ""` | `Received by Anita Rao.` |
| `Kind == came_back` and `ShortQuantity > 0` | `Won't come back: 5 chairs broke during setup near the stage. Ravi informed.` — the exact line `/out` renders, prefixed `Won't come back:` or `Still expected back:` by disposition |
| `RecordDeleted` and `Kind != deleted` | **`This entry was deleted later.`** The main line is also greyed and struck through, matching the tombstone treatment on `/inwards`, and the `Go to this entry` link stays because the deleted row is still on that tab. |

**That sentence is required, not decorative.** Without it the row for a deleted arrival
reads `500 chairs came in from Sharma Tent House` — word for word what a live arrival
reads — and the only thing separating them is strikethrough. A reader hunting a wrong
number, skimming, on a laptop screen at a gathering, will count those 500 chairs. The
one page whose job is explaining a number that looks wrong cannot rely on typography
alone to say a thing did not happen.

The inward comparison is a date-only string against a timestamp's **local calendar
date** — not a timestamp comparison. The other two compare timestamps for exact
equality.

#### Reusing the correction and deletion wording

`11-corrections.spec.md` owns twelve per-field line shapes and two empty-side cases, and
defines the rendered line as a phrase followed by ` by <name>, <clock>`. The log's
`Who` and `Time` columns already carry that suffix, so the log renders **the phrase
only**, through the same function:

```go
func changePhrase(c register.Change) string  // "Changed it from 500 chairs to 50 chairs"
func changeLine(c register.Change) string    // changePhrase(c) + " by " + c.By + ", " + clock(c.At)
```

`changeLine` is what `/inwards` and `/out` already render (`10-views.spec.md`) and what
`edit.html` renders (`11-corrections.spec.md`); this spec requires only that the phrase
be a separately callable function so the log does not become a second source of truth
for those fourteen sentences. Both live in `internal/web/corrections.go`. The same
applies to the deletion reason: the log renders `Deletion.Reason` verbatim, never a
re-worded copy.

#### Two new template helpers

- `daystamp(t)` → `Thursday, 3 September` — the day headings. Used only here, so it
  lives in `internal/web/log.go` and is registered there.
- `shortdate(t)` → `3 September` — the `Received on ...` line. This is the same
  rendering `11-corrections.spec.md` uses in
  `Changed the date from 3 September to 4 September`, and spec 11 is built first, so
  **`shortdate` belongs to `04-server-and-shell.spec.md` alongside `longstamp` and
  `clock`**, not to this spec. Spec 04 is already being amended for the fifth tab; this
  is the second line of that amendment. Spec 11 and this spec both call it, and neither
  formats a date itself.

#### When nothing matches

No result count is ever displayed — a count is the one place on this page where
`1 lines` could appear, and there is no pluralisation logic (index decision 10).

Two sentences, chosen by which filters are active:

- only `day` is set:
  `Nobody wrote anything down on Thursday, 3 September.` and beneath it, in `sm` type,
  `Pick another day, or tap Every day.`
- any of `kind`, `q` or `productId` is also set:
  `Nothing matches what you picked.` and beneath it,
  `Tap Every day, Everything, Anybody and Any product.` — all four clearers are named.
  Naming only two produces advice that can fail: a reader with a person filter still set
  taps both, sees nothing change, and concludes the page is broken.

#### Read-only

No control on this page writes anything. Enforced the way `10-views.spec.md` enforces it
on the Suppliers tab, adapted for the one `GET` form the filters need:

- the route is registered `GET /log` only, so `POST /log` returns **405** from Go's
  pattern router — not the 404-with-shell of `04-server-and-shell.spec.md`, which
  applies to unmatched paths;
- `log.html` contains no `method="post"`;
- `internal/web/log.go` contains no `store.Update`.

### The fifth tab

The chrome bar's tabs become five, in this order:
**Stock**, **Out with people**, **Stuff came in**, **Suppliers**, **Who did what**,
linking to `/stock`, `/out`, `/inwards`, `/suppliers`, `/log`.

`Who did what` rather than `Log`: the other four labels are the desk's own words for the
thing behind them, and `Log` is record-keeping vocabulary that a second-language reader
at a store desk has no reason to know. It is also the user's own phrasing of what the
page is for — "if someone makes changes and we want to check who did it".

## Files to create or modify

Create:

- `/home/asim/Projects/inventory-management/internal/register/log.go`
- `/home/asim/Projects/inventory-management/internal/register/log_test.go`
- `/home/asim/Projects/inventory-management/internal/web/log.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/log.html`
- `/home/asim/Projects/inventory-management/internal/web/log_test.go`

Modify:

- `/home/asim/Projects/inventory-management/internal/web/server.go` — register `GET /log`
- `/home/asim/Projects/inventory-management/internal/web/templates/layout.html` — the
  fifth tab
- `/home/asim/Projects/inventory-management/internal/web/people.go` — the `scope=log`
  population, and suppressing the `+ New person named` row in that scope
- `/home/asim/Projects/inventory-management/internal/web/corrections.go` — split
  `changeLine` into `changePhrase` + `changeLine`, and factor the three record-in-words
  sub-heading shapes into `entryName`; no behaviour change on any existing screen
- `/home/asim/Projects/inventory-management/internal/web/templates/inwards.html` —
  `id="<inwardID>"` on each row
- `/home/asim/Projects/inventory-management/internal/web/templates/out.html` —
  `id="<issueID>"` on each outstanding line, `id="<returnID>"` on each `Came back` line
- `/home/asim/Projects/inventory-management/specs/00-index.spec.md` — build order and
  file list

## Required tests

Clock `2026-09-03T18:10:00+05:30` (**T4**) over the **T3** register, Suresh Kumar on
duty, unless a test says otherwise.

### The builder — `internal/register/log_test.go`

`TestLogEntriesAtT0` — `LogEntries(WalkthroughT0())` returns exactly 21 entries: 5
`product_added`, 3 `person_added`, 6 `came_in`, 7 `went_out`, 0 of everything else. No
entry has `Kind == "shift_started"` — that kind does not exist.

`TestLogOrderIsNewestRecordedFirst` — over T3, the first six entries are, in order,
`RET-0001` (18:05), `ISS-0008` (14:18), `INW-0007` (10:42), `ISS-0005` (09:40),
`ISS-0003` (09:40) — the whole of 3 September — and then `ISS-0006`, the newest entry of
2 September at 12:15. This one test pins newest-first, the same-instant tie-break and
the day boundary together.

`TestLogTieBreaksAcrossKinds` — `INW-0001` (`RecordedAt 2026-09-01T09:15`) and
`ISS-0002` (`RecordedAt 2026-09-01T09:15`) collide exactly. `ISS-0002` comes first,
because `went_out` outranks `came_in` and rank sorts descending. This is the fixture's
only cross-kind collision.

`TestLogTieBreaksWithinAKind` — `ISS-0003` and `ISS-0005` are both recorded
`2026-09-03T09:40`; `ISS-0005` comes first. The five `product_added` entries, all at
`2026-09-01T08:00:00`, appear as `PRD-0005`, `PRD-0004`, `PRD-0003`, `PRD-0002`,
`PRD-0001`. The three `person_added` entries have distinct times and appear
`STF-0003` (07:40), `STF-0002` (07:35), `STF-0001` (07:30), below all five products.

`TestLogIsDeterministic` — build the log from the same register 50 times and assert all
50 slices are identical field-for-field. Then build it from a copy whose `Products`,
`Staff`, `Inwards` and `Issues` slices have been reversed, and assert the result is
identical to the first. Ordering may not depend on input order or on map iteration.

`TestLogSortsOnRecordedNotHappened` — over a T2 copy whose `ISS-0008` has
`IssuedAt 2026-09-03T09:00` and `RecordedAt 2026-09-03T14:18` (backdated at the desk),
`ISS-0008` still sorts above `INW-0007` (recorded 10:42) and above `ISS-0003`
(09:40). Its `HappenedAt` is 09:00 and its `At` is 14:18.

`TestLogShowsCorrectionsAsTheirOwnEntries` — over a T1 copy where `INW-0007` was changed
from 500 to 50 by Suresh Kumar at 10:45, the log has both the `came_in` entry at 10:42
and a `corrected` entry at 10:45, the corrected one first, its `Change` pointing at that
`Change` and its `RecordID` reading `INW-0007`.

`TestLogKeepsTwoChangesFromOneEditInFieldOrder` — the register from spec 11's
`TestFixInwardSupplierAndChallan`: two `Change` entries on `INW-0007` at one instant,
supplier then challan. The log lists the supplier change **above** the challan change —
`ChangeIndex` ascending, the deliberate exception to newest-first.

`TestLogShowsDeletedRecordsAndTheirDeletion` — over a T1 copy where `INW-0002` was
deleted by Suresh Kumar at 10:47 with reason `Entered twice by mistake.`: the log has
the original `came_in` entry for `INW-0002` at `2026-09-02T08:30` with
`RecordDeleted == true`, **and** a separate `deleted` entry at 10:47 carrying that
`Deletion`. Nothing about `INW-0002` is missing from the list, and `CameIn(Chairs)` is
still 390 — the log sees it, the arithmetic does not.

`TestLogNamesTheActorPerKind` — over T3: `INW-0007`'s `came_in` entry has
`Who == "Suresh Kumar"`; `ISS-0008`'s `went_out` entry has `Who == "Anita Rao"` and
`PersonName == "Ravi Menon"`; `RET-0001`'s `came_back` entry has
`Who == "Imran Sheikh"` and `PersonName == "Ravi Menon"`; the `person_added` entry for
`STF-0002` has `Who == "Suresh Kumar"`, `PersonName == "Anita Rao"` and
`PersonMobile == "99001 34562"`; the `person_added` entry for `STF-0001` has
`Who == ""`, because nobody was on duty when the first person was added; every
`product_added` entry has `Who == "Suresh Kumar"`.

`TestFilterByDay` — over T3 with `loc` IST, `LogFilter{Day: "2026-09-03"}` returns
exactly five entries (RET-0001, ISS-0008, INW-0007, ISS-0005, ISS-0003);
`{Day: "2026-09-01"}` returns fourteen (the three `person_added` rows, the five
`product_added` rows, INW-0001, INW-0003, INW-0004, ISS-0001, ISS-0002, ISS-0007 —
assert the exact IDs); `{Day: "2026-09-02"}` returns five (INW-0002, INW-0005, INW-0006,
ISS-0004, ISS-0006); `{Day: ""}` returns all 24.

`TestFilterByKind` — over T3, `{Kinds: [LogCameIn]}` returns seven entries, all
inwards; `{Kinds: [LogCameBack]}` returns exactly `RET-0001`;
`{Kinds: [LogPersonAdded]}` returns exactly `STF-0003`, `STF-0002`, `STF-0001`.

`TestFilterByProduct` — over T3, `{ProductID: "PRD-0001"}` returns exactly nine entries
— `PRD-0001`, `INW-0001`, `INW-0002`, `INW-0007`, `ISS-0001`, `ISS-0002`, `ISS-0003`,
`ISS-0008`, `RET-0001` — so the `product_added` row for Chairs is included and **no**
`person_added` row is.

`TestFilterByPerson` — over T3: `{Query: "98861"}` returns `ISS-0003`, `ISS-0005`,
`ISS-0008` and `RET-0001` — everything about Ravi Menon, actor or subject.
`{Query: "Imran"}` returns `ISS-0002`, `ISS-0007`, `INW-0005`, `RET-0001` and the
`person_added` row `STF-0003` — the three he recorded, the one he took back, and the day
he was put on the list. `{Query: "cater"}` returns Ravi's three issues.
`{Query: "Ravi Menon"}` and `{Query: "9886140023"}` return the same four rows as
`98861`.

`TestFilterExcludesEntriesWithNoValueForTheDimension` — over T3,
`{ProductID: "PRD-0001", Query: "Anita Rao"}` returns exactly `ISS-0008`: the
`person_added` row for Anita Rao is dropped for having no product, and every chairs row
that does not name her — the `product_added` row for Chairs among them, whose `Who` is
Suresh Kumar — is dropped for having no matching person. `{ProductID: "PRD-0002"}`
returns no `person_added` row at all.

`TestFiltersCombine` — over T3,
`{Day: "2026-09-03", Kinds: [LogWentOut], Query: "98861"}` returns exactly `ISS-0003`,
`ISS-0005` and `ISS-0008`; adding `ProductID: "PRD-0001"` drops `ISS-0005`.

`TestPeopleInLogIncludesStaffAndSettledTakers` — over a T3 copy with a further return
settling Ravi Menon's last 5 chairs and 2 round tables, `FindPeople(reg, "98861")`
returns nothing (spec 03's rule) while `FindPeopleInLog(reg, "98861")` still returns
Ravi Menon. `FindPeopleInLog(reg, "Anita")` returns Anita Rao, whom `FindPeople` can
never return because staff hold nothing. `PeopleInLog` over T3 returns exactly seven
people — Anita Rao, Farida Begum, Imran Sheikh, Joseph D'Cruz, Lakshmi Iyer, Ravi Menon,
Suresh Kumar — in that order, each once.

`TestPeopleInLogIncludesTakersOnDeletedRecords` — over T0 with `ISS-0001` tombstoned,
`FindPeopleInLog(reg, "Lakshmi")` still returns Lakshmi Iyer.

### The page — `internal/web/log_test.go`

`TestLogTabIsInTheChromeBar` — `GET /stock` contains all five labels `Stock`,
`Out with people`, `Stuff came in`, `Suppliers`, `Who did what`, and a link to `/log`.
`GET /log` returns 200 and its `Who did what` tab carries the `on` class.

`TestLogDefaultsToToday` — `GET /log` at T4 over T3 renders the heading
`Thursday, 3 September` and exactly five rows, top to bottom:

```
6:05 pm   Imran Sheikh   45 chairs came back from Ravi Menon
2:18 pm   Anita Rao      10 chairs went out to Ravi Menon
10:42 am  Suresh Kumar   500 chairs came in from Sharma Tent House
9:40 am   Suresh Kumar   2 round tables went out to Ravi Menon
9:40 am   Suresh Kumar   40 chairs went out to Ravi Menon
```

and contains none of `INW-0001`'s row, no `2 September` heading, no
`added to the product list` and no `added to the people list`.

`TestLogShortfallLineMatchesTheOutTab` — the `RET-0001` row carries the `sm` line
`Won't come back: 5 chairs broke during setup near the stage. Ravi informed.`, byte-for-byte
the line `GET /out` renders for the same return.

`TestLogLinksToTheRecordOnItsTab` — the `RET-0001` row contains `/out#RET-0001`, the
`ISS-0008` row `/out#ISS-0008`, the `INW-0007` row `/inwards#INW-0007`, each with the
link text `Go to this entry`. The body contains no `/entry/` anywhere. `GET /inwards`
contains `id="INW-0007"` and `GET /out` contains `id="ISS-0008"` and `id="RET-0001"`.

`TestLogEveryDayShowsEveryDay` — `GET /log?day=all` renders three day headings,
`Thursday, 3 September`, `Wednesday, 2 September` and `Tuesday, 1 September`, in that
order, and contains `Chairs added to the product list.` and
`Anita Rao added to the people list.` It contains no row anywhere reading
`came on duty` — there are no shift lines in this log.

`TestLogRowForTheFirstPersonHasNoWho` — under the `Tuesday, 1 September` heading on
`GET /log?day=all`, the row `Suresh Kumar added to the people list.` has
`nobody wrote it down` in its `Who` column, while `Anita Rao added to the people list.`
has `Suresh Kumar`. No placeholder name is invented for the first person on the list.

`TestLogShowsBothTimesWhereTheyDiffer` — over the register from spec 11's
`TestFixIssueTime` (`ISS-0008` has `IssuedAt 13:05`, `RecordedAt 14:18`), the `ISS-0008`
row sits under the `2:18 pm` time and carries the `sm` line `Taken at 1:05 pm.`
Over unmodified T3, no row carries a `Taken at` or `Came back at` line, because every
fixture record was recorded at the moment it happened.

`TestLogShowsADifferentReceivedDate` — over a T1 copy whose `INW-0007` has
`ReceivedOn: "2026-09-04"` while `RecordedAt` stays `2026-09-03T10:42`, the row carries
`Received on 4 September.` With `ReceivedOn: "2026-09-03"` it carries no such line —
the comparison is against the recorded timestamp's calendar date, not against the
timestamp.

`TestLogShowsADifferentReceiver` — over a T1 copy whose `INW-0007` has
`ReceivedBy: "Anita Rao"` and `RecordedBy: "Suresh Kumar"`, the `Who` column reads
`Suresh Kumar` and the `sm` line reads `Received by Anita Rao.`

`TestLogCorrectionPhraseMatchesTheInwardsTab` — over the T1 copy where `INW-0007` was
changed from 500 to 50 by Suresh Kumar at 10:45: `/log` shows a `10:45 am` row whose
`Who` is `Suresh Kumar`, whose main line is `50 chairs from Sharma Tent House` and whose
`sm` line is exactly `Changed it from 500 chairs to 50 chairs` — with no `by Suresh Kumar` and
no `10:45 am` inside it, because the columns already carry them. `/inwards` for the same
`Change` renders `Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am`, and the
log's line is a prefix of it.

`TestLogDeletionRow` — over the T1 copy where `INW-0002` was deleted by Suresh Kumar at
10:47 with reason `Entered twice by mistake.`: `/log?day=all` shows a `10:47 am` row
whose `sm` line is `Deleted — Entered twice by mistake.`, and separately shows the
original `310 chairs that came in` row for `INW-0002` on 2 September, struck through.
Neither row is missing.

`TestLogFilterButtonsKeepOtherFilters` — on `GET /log?day=all&q=Imran`, the `Came back`
link's href contains `day=all`, `q=Imran` and `kind=came_back`, and the `Every day`
link is marked `on`.

`TestLogFilterByPersonFromThePicker` — `GET /log?day=all&q=98861` returns rows for
`ISS-0003`, `ISS-0005`, `ISS-0008` and `RET-0001` and no others.
`GET /api/people?scope=log&q=Imran` returns Imran Sheikh, and the rendered suggestion
row is `Imran Sheikh · 90080 77213`. `GET /api/people?q=Imran` (no scope) returns
nothing, unchanged.

`TestLogPickerOffersNoNewPerson` — `GET /api/people?scope=log&q=Meera` renders no
`+ New person named Meera` row, while `GET /api/people?q=Meera` still does.

`TestLogEmptyForADayWithNothingOnIt` — `GET /log?day=2026-08-30` contains
`Nothing happened on Sunday, 30 August.` and `Pick another day, or tap Every day.`

`TestLogEmptyWithFiltersOn` — `GET /log?day=2026-09-03&kind=came_back&q=Lakshmi Iyer`
contains `Nothing matches what you picked.` and
`Tap Every day and Everything to see more.` and not `Nothing happened on`.

`TestLogIgnoresRubbishParameters` — `GET /log?day=yesterday&kind=exploded&productId=PRD-9999`
returns 200, renders the default day, and its body contains no `invalid`, no `error` and
no Go type name.

`TestLogIsReadOnly` — `POST /log` returns 405. `GET /log` over T3 leaves the file on
disk byte-for-byte unchanged, and a fresh `store.Open` of the directory returns a
register `reflect.DeepEqual` to the one before the request.

`TestLogNeedsAShift` — with `OnDutyStaffID` empty, `GET /log` returns 303 to `/shift`.

`TestLogNeverAbbreviatesAName` — over T3, the `/log` body contains, verbatim,
`10 chairs went out to Ravi Menon`, `40 chairs went out to Ravi Menon` and
`45 chairs came back from Ravi Menon`; and it contains none of the four substrings
` to Ravi<`, ` to Ravi `, ` from Ravi<`, ` from Ravi ` — the shapes an abbreviated name
would produce in a rendered line. The verbatim assertions are what enforce the rule; the
four negatives catch the regression. The remark
`5 chairs broke during setup near the stage. Ravi informed.` is quoted from the record
and is not a rendered line, so it trips none of them.

`TestLogShowsNoCount` — this one test does not use `assertContains`. It runs
`regexp.MustCompile` over the pattern `[0-9]+ (lines|rows|entries|results|records)` against
`html.UnescapeString(string(body))` for `/log` and `/log?day=all`, and asserts no match.
A count is the one place on this page where `1 lines` could appear.

## Acceptance criteria

1. `go test ./internal/register/ -run TestLog -count=1` and
   `go test ./internal/register/ -run 'TestFilter|TestPeopleInLog' -count=1` pass, with
   all eighteen builder tests above.
2. `go test ./internal/web/ -run TestLog -count=1` passes with all twenty-one page tests
   above.
3. `go test ./... -race -count=1` passes.
4. **The log is the only tombstone reader in the package, and it is not in `arith.go`:**
   `grep -nE 'range r\.(Inwards|Issues|Returns)' internal/register/arith.go` still
   returns nothing (spec 03 criterion 6, unchanged), and
   `grep -cE 'range r\.(Inwards|Issues|Returns|Products)' internal/register/log.go` is
   at least 4.
5. **No clock and no I/O in the builder:**
   `grep -n 'time.Now()\|os\.\|net/http' internal/register/log.go` returns nothing.
6. **Read-only:** `grep -n 'method="post"' internal/web/templates/log.html` returns
   nothing; `grep -n 'store.Update' internal/web/log.go` returns nothing;
   `grep -n '"POST /log"' internal/web/server.go` returns nothing.
7. **No second source of truth for the correction sentences:**
   `grep -cE 'Changed (how many|the )' internal/web/log.go internal/web/templates/log.html`
   returns 0 for both files — every correction sentence comes from `changePhrase` in
   `corrections.go`.
8. **No parallel log table:**
   `grep -rniE 'auditLog|activityLog|LogRecord|logEntries *\[\]' internal/register/model.go`
   returns nothing. `Register` may contain the schema-3 `Disposals` and encrypted
   `Finance` fields required by specs 17–20, but still contains no stored activity-log
   slice. (The runtime half—the file being untouched by a visit—is `TestLogIsReadOnly`,
   not a criterion here.)
9. **No edit route is reachable from this page:**
   `grep -n '/entry/' internal/web/templates/log.html` returns nothing.
10. `grep -rniE 'password|login|authenticate|permission|audit trail' internal/web/log.go internal/web/templates/log.html`
    returns nothing. This page attributes; it does not police.
11. `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` succeeds and
    `go list -deps ./... | grep -v '^storeregister' | grep '\.'` prints nothing.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/register/ -run 'TestLog|TestFilter|TestPeopleInLog' -count=1 -v
go test ./internal/web/ -run 'TestLog' -count=1 -v
go test ./... -race -count=1
go vet ./...
grep -n 'store.Update' internal/web/log.go            # must print nothing
grep -n '/entry/' internal/web/templates/log.html     # must print nothing
grep -nE 'range r\.(Inwards|Issues|Returns)' internal/register/arith.go   # must print nothing
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
```

## Amendments already applied to other specs

All of these are in place; they are listed so an implementer knows which other specs
moved and why, and so a reviewer can check that they did.

- **`01-data-model.spec.md`** — `Product` gains `CreatedBy`; `Staff` gains `CreatedAt`
  and `CreatedBy`; the field-conventions list states that `CreatedBy` is empty for the
  first staff member on a fresh register; the T0 staff list becomes a table carrying the
  new values (`STF-0001` at 07:30 with an empty `CreatedBy`, `STF-0002` at 07:35 and
  `STF-0003` at 07:40, both by Suresh Kumar); the T0 inward table's `ReceivedBy` column
  becomes `ReceivedBy / RecordedBy` with a sentence saying they are equal throughout, so
  the log's `Who` column is defined for all six; and a fourteenth test,
  `TestFixtureCarriesProvenance`, asserts the lot.
- **`04-server-and-shell.spec.md`** — `GET /log` in the routing table; four tabs become
  five; `TestShellShowsOnDutyName` asserts five labels; `TestRecoveryWarningShowsOnEveryPage`
  includes `/log`; `shortdate` joins `longstamp` and `clock`, with two cases in
  `TestLongstampMatchesWalkthrough`.
- **`05-shift-and-people.spec.md`** — `POST /shift/person` sets `CreatedAt` and
  `CreatedBy`, empty when nobody is on duty; `TestAddPerson` asserts both, and
  `TestFirstPersonOnAFreshRegisterHasNoCreatedBy` is added.
- **`06-products.spec.md`** — `POST /product/new` sets `CreatedBy` from the on-duty
  person, which the shift guard makes never empty; `TestCreateProductAppends` asserts
  it.
- **`08-issue.spec.md`** — `/api/people` gains the optional `scope=log`; the
  `+ New person named <typed>` row is "always present except in `scope=log`";
  `TestPersonPickerAlwaysOffersANewPerson` asserts its absence in that scope.
- **`10-views.spec.md`** — inward rows carry `id="<inwardID>"`, outstanding lines
  `id="<issueID>"`, `Came back` lines `id="<returnID>"`, with `TestRowsCarryAnchors`.
- **`11-corrections.spec.md`** — `entryName`, `changePhrase` and `changeLine` are named
  and located; the `date received` line renders through spec 04's `shortdate`.

## Settled

1. **Provenance fields added, so both "a product was added" and "a person was added" are
   derivable.** `Staff` gains `CreatedAt time.Time` and `CreatedBy string`; `Product`
   gains `CreatedBy string` beside its existing `CreatedAt`. `CreatedBy` is the on-duty
   person's name at the time. For the very first staff member on a fresh register there
   is nobody on duty, so `CreatedBy` is empty and the row renders with no who — no
   placeholder name is invented. Specified in `01-data-model.spec.md`, populated by
   `05-shift-and-people.spec.md` (`POST /shift/person`) and `06-products.spec.md`
   (`POST /product/new`).

2. **No shift history, and therefore no shift lines.** No `Shifts` slice, no
   `shift_started` kind. Reasoning in the contract above: every entry already names the
   person who made it, and a log that shows one instance of a category it cannot show
   historically teaches the reader it is complete when it is not. Revisit only if
   somebody asks who was at the desk during a stretch when nothing was entered.

3. **A corrected row names the record as it reads now.** See item 4 below for the shape
   of the compromise; replaying each record backwards through its `Changes` is more
   machinery than the confusion is worth.

4. **`Who did what` is reachable only from the tab bar.** No `Who entered this?`
   shortcut on `/inwards`, no jump-to-log link anywhere else.

5. **The five tabs fit.** Measured against the CSS this spec inherits verbatim
   (`design/store-register.html`): `.screen` caps at `52rem` (832 px), `.tabs` has
   `1.25rem` padding each side, `.tab` has `0.85rem` padding each side at `0.83rem`
   Archivo semibold, gaps `0.4rem`. Five labels totalling 54 characters come to roughly
   600 px including padding and gaps — around 230 px of slack. `.tabs` is
   `display:flex` with the default `nowrap` and `.tab` is `white-space:nowrap`, so
   nothing can wrap. `Who did what` needs no shortening; if a future sixth tab arrives,
   the shortest label keeping the desk's voice is `Who did it`.

## Open

1. **The failure mode of item 3 above, for the record.** On the T1 register corrected at
   10:45, two adjacent rows read

   ```
   10:45 am  Suresh Kumar  50 chairs from Sharma Tent House
                           Changed it from 500 chairs to 50 chairs
   10:42 am  Suresh Kumar  500 chairs came in from Sharma Tent House
   ```

   — one record, two quantities, three lines apart, on the page whose whole job is
   explaining a number that looks wrong. Accepted: the change phrase directly beneath
   states the before and after, so the pair explains itself once read. Recorded here so
   that if a reader at the desk trips over it, the fix is known — replay each record
   backwards through its `Changes` — and nobody has to rediscover the cause.

2. **The exact wording of the seven main lines, the seven `sm` lines and the four empty
   sentences.** Written here in the register's plain voice and asserted verbatim in the
   tests, so a reword touches this spec and its tests and nothing else. With
   `plain-language-reviewer`; nothing waits on it.
