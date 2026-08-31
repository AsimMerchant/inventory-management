# Store Register — Spec Index

Non-normative. Every rule lives in a numbered spec; this page is a map and a list of
the questions that must be settled before or during the build.

The approved design is the walkthrough at
`scratchpad/store-register.html`. Every spec traces to it. Where it is silent, the spec
says so under its own `## Open` rather than inventing an answer, and those questions are
gathered at the bottom of this page.

## Build order

Each spec depends only on the ones above it.

| # | Spec | What it settles | Roughly |
|---|---|---|---|
| 01 | `01-data-model.spec.md` | Types, the JSON file format, **the walkthrough fixture and the named timepoints T0–T4** | half a day |
| 02 | `02-persistence.spec.md` | Atomic save, `.bak` fallback, crash recovery | half a day |
| 03 | `03-stock-arithmetic.spec.md` | On hand, outstanding per person, what came in from each supplier, `Validate`, the two refusals | one day |
| 04 | `04-server-and-shell.spec.md` | Loopback bind, port hunt, browser launch, layout, tabs | half a day |
| 05 | `05-shift-and-people.spec.md` | Who is on duty; auto-stamping every entry | quarter day |
| 06 | `06-products.spec.md` | The picker and the never-free-typed invariant | half a day |
| 07 | `07-inward.spec.md` | Stuff came in | half a day |
| 08 | `08-issue.spec.md` | Someone is taking; the shared person picker and `/api/people` | one day |
| 09 | `09-return.spec.md` | Someone is returning, partial returns, the required remark | one day |
| 10 | `10-views.spec.md` | Stock, Out with people, Stuff came in, Suppliers | half a day |
| 11 | `11-corrections.spec.md` | Fixing and deleting a wrong entry, with guards and an audit line | one day |
| 12 | `12-activity-log.spec.md` | Who did what — one chronological list derived from the records, with day, person, kind and product filters | one day |

Spec 01 must be built first: it contains the fixture that every other spec's test cases
are written against. A test that says "at T1, 890 chairs are on hand" is meaningless
until `register.WalkthroughT0()` exists.

## The shape of the thing

```
storeregister/
  go.mod                       module storeregister, go 1.27
  main.go                      startup, port, browser
  internal/register/           model, ids, fixture, arithmetic, validate — no I/O, no clock
  internal/store/              atomic save and load             — no HTTP
  internal/web/                handlers, templates, static      — no arithmetic
  specs/                       00-index, 01-data-model, 02-persistence, 03-stock-arithmetic,
                               04-server-and-shell, 05-shift-and-people, 06-products,
                               07-inward, 08-issue, 09-return, 10-views, 11-corrections,
                               12-activity-log
```

**The chrome bar has five tabs**, in this order: `Stock`, `Out with people`,
`Stuff came in`, `Suppliers`, `Who did what` — the fifth from
`12-activity-log.spec.md`, and `04-server-and-shell.spec.md` amended to match.

Standard library only. No cgo. One data file, `store-register.json`, beside the
executable.

## Three principles that settle whole classes of question

**People records may be sloppy. Product records may not.**

A duplicate or misspelt *person* is acceptable and must never block or interrupt
anybody. No dedup confirmation, no "did you mean", no merge prompt, no warning on a
near-match. Two spellings of one man are two rows, and the staff sort it out between
themselves — by the end of their shifts they know each other. A duplicate *product* is
unacceptable, and the guards in `06-products.spec.md` stay exactly as written: the
case-fold refusal and the near-duplicate prefix confirmation.

The asymmetry is not an inconsistency. A duplicate person is a cosmetic problem a human
can see and fix. A duplicate product is an arithmetic problem that silently halves the
number the whole register exists to produce, and nobody notices until the count is
already wrong. Where the two rules disagree, speed at the desk wins for people and
correctness wins for products.

**Nobody logs in.** There is no password, no PIN, no authentication and no permission
anywhere in the program. Tapping a name on the shift screen puts that name on the
entries made afterwards; that is attribution, not security. Any spec, screen or field
implying otherwise is a defect — see `05-shift-and-people.spec.md`, whose acceptance
criteria grep the whole tree for the word.

**The store's job ends when the stock is back in the store.** Getting rented goods back
to the people who own them is somebody else's work, done outside this software. Nothing
in here tracks a debt, a settlement or a rupee.

## Conventions binding on every spec

These are normative and are stated here once so no spec re-decides them.

1. **Apostrophes and dashes in user-visible text are straight ASCII.** The walkthrough
   is typeset with curly quotes (`Won’t`, `don’t`, `you’d`); the shipped strings use
   `'`. Em dashes (`—`) and the middle dot (`·`) are kept as the walkthrough has them,
   because they carry layout meaning. So: `Won't come back — broken or lost`.
2. **HTML assertions unescape first.** `html/template` renders `'` as `&#39;` and `&`
   as `&amp;`, so `Joseph D'Cruz` and `Stage & Sound` never appear literally in a
   response body. Every test that asserts a page *contains* a sentence compares against
   `html.UnescapeString(string(body))` through one shared helper,
   `internal/web.assertContains(t, body, want string)`. Written once, in
   `internal/web/testhelp_test.go`.
3. **Fixture times use `time.FixedZone("IST", 5*3600+30*60)`**, never
   `time.LoadLocation`. `LoadLocation` needs the tzdata files, which are absent from a
   `CGO_ENABLED=0 GOOS=windows` binary unless `time/tzdata` is imported, and a fixed
   zone keeps the timestamp tests deterministic on a developer machine in any
   timezone.
4. **Greps in acceptance criteria exclude test files** unless the criterion says
   otherwise: `grep -rn --include=*.go --exclude=*_test.go`. Test files legitimately
   contain the walkthrough's names, and may contain a forbidden literal in an
   assertion.

## Standing constraints, restated

Every spec assumes these and none of them may be traded away:

- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` succeeds at every commit.
- The listener binds `127.0.0.1`, never `0.0.0.0`, never `:port`.
- A crash mid-save leaves either the previous good file or the new good file.
- Stock is pooled per product; issues and returns never reference an inward record.
- Product names are picked, never typed into a record.
- Over-issue and over-return are refused.
- A short return cannot be saved without a disposition and a remark.
- Short items stay outstanding against the person. Nothing is ever written off.
- There is no settlement, no payment, no money and no supplier debt anywhere in the
  program.
- A wrong entry can be corrected or deleted, never silently: every correction keeps what
  it used to say, who changed it and when, and a deleted record stays in the file as a
  tombstone that counts towards nothing.
- No correction may leave the register in a state `register.Validate` complains about.
- Every arithmetic function skips deleted records. There is no function that sees them.

## Whole-build verification

```
cd /home/asim/Projects/inventory-management
go test ./... -race -count=1
go vet ./...
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/register.exe .
go list -deps ./... | grep -v '^storeregister' | grep -v '^vendor/' | grep -v '^crypto/internal' | grep '\.'   # must print nothing
grep -rn --include=*.go --exclude=*_test.go '0\.0\.0\.0' .   # must print nothing
grep -rniE --include=*.go --include=*.html 'still owed|given back|settle' .   # nothing
grep -rniE --include=*.go --include=*.html --include=*.js 'password|login|authenticate' .   # nothing
```

## Open questions

The spec-writer raised 26. The orchestrator settled 23; the user settled the remaining
three. Nothing is outstanding — everything below is a **normative decision**, listed so
no spec and no implementer re-opens it.

### Settled by the user

**1. "Given back" — removed, not built.** *"We don't know and maybe we don't care. As
someone who inventories, we care about: I got 500 chairs, I issued 500, I want 500 at
the end of the event. That's it. There are other people who handle this going back to
the supplier."*

There is no fourth event. The `givenBack` parameter, the `Given back` column, the
`Still owed` column, the `All back` pill and the `SupplierKey` type are all **deleted**
from specs 03 and 10. The Suppliers tab is now a plain read-only record: supplier,
product, how many they sent, rent or purchase. The `won't come back` figure survives as
`5 broken or lost` — a note on the record, not a qualifier on a debt.

**2. Correcting a mistake — built.** *"Yes, should be able to edit the entry from
software."* Specified in `11-corrections.spec.md`: any inward, issue or return can be
corrected or deleted; every correction keeps an audit line in plain words
(`Changed it from 500 chairs to 50 chairs by Suresh Kumar, 10:45 am`); every guard is the single
`register.Validate` checker rather than a hand-derived formula per field; reachable from
the `Fix this` links on the lists, never from an admin screen. Supersedes old items 2
and 6.

**3. Person identity — name and mobile together, one search field, and no nagging.**
*"We will use full names and mobile number and ideally I would have users type in either
mobile number or full name in same field."* … *"We don't mind if same person has 2
entries like Ravi verma or ravii varma."* … *"Same spelling of full name should simply
show existing name along with mobile number because 2 people can have same first and
last names."*

A person is `FoldKey(name)` + `MobileKey(mobile)` together; no mobile means keyed on the
name alone. One input finds by name, mobile or department. The suggestion list shows
`<Name> · <Mobile> · <Department>` and always ends with `+ New person named <typed>`. It
offers and never insists — see the first principle at the top of this page. Supersedes
old items 3 and 16.

### Settled by the orchestrator — normative

| # | Decision |
|---|---|
| 4 | Chairs split 390 rent from Sharma + 310 purchase at T0. Fixture-only; affects no shipped behaviour. |
| 5 | A product with both rent and purchase inwards shows `Rent` if any inward is rent. |
| 7 | No product rename in v1. |
| 8 | Near-duplicate product names: confirmation step on a shared 4-character prefix. |
| 9 | Supplier names stay free text with a case-fold guard, so `Sharma tent house` reuses `Sharma Tent House`. |
| 10 | No pluralisation logic. Rephrase any label that would read `1 chairs` rather than build a pluraliser. |
| 11 | The `productWord` casing rule as derived in spec 07. |
| 12 | The amber banner renders the real product name — `2 Round tables`, not `2 tables`. |
| 13 | Editable timestamps are labelled `Time taken` and `Time returned`. |
| 14 | No explicit end-of-shift and no removing people. Clicking the on-duty name in the chrome bar returns to `/shift`. |
| 15 | A fresh install ships empty. The walkthrough's three names are fixture data. The venue adds its own staff on day one. |
| 17 | "Out over 2 days" counts lines, not items. |
| 18 | "Out right now" includes every product. |
| 19 | Supplier rows sort alphabetically. |
| 20 | The "Out with people" and "Stuff came in" tab layouts as recommended in spec 10. |
| 21 | Port 8765, hunting upward to 8785. |
| — | **`Product` carries `CreatedBy`; `Staff` carries `CreatedAt` and `CreatedBy`.** Set from the on-duty name, empty for the first staff member on a fresh register, with no placeholder substituted. Added so `12-activity-log.spec.md` can say who added a product or a person. Specified in spec 01, populated by specs 05 and 06. |
| — | **No shift history and no shift lines in the log.** No `Shifts` slice. Every entry already names the person who made it, and a log showing one instance of a category it cannot show historically would teach the reader it is complete when it is not. Revisit only if somebody asks who was at the desk while nothing was entered. |
| — | **A shift is live only on its own calendar day.** Reopening the laptop the next morning returns to `/shift` rather than stamping the new person's entries with yesterday's name. Specified in spec 05. Supersedes spec 01 Open item 4. |

Return-screen behaviour, settled together because they are one question:

| # | Decision |
|---|---|
| 2 | Allocation is oldest issue first. |
| 3 | Selection is by product, not by line. The desk hands back 45 chairs; the software decides which lines they come off. |
| — | One product per return entry, as spec 09 recommends. |

Item 19 (supplier row ordering) still stands: rent rows first, alphabetical by supplier
then product, then the non-rent rows. Items 6 and 16 are superseded by user decisions 2
and 3 above.

### Still open — small, and none of them blocks a build

Each is recorded in its own spec with a recommendation. They can be answered while the
code is being written.

| Spec | Question |
|---|---|
| 01 | A mobile typed with a country code (`+91 98861 40023`) keys as a different person. Recommend leaving it. |
| 03 | The same person entered once with a mobile and once without stays two rows. Accepted, not guarded. |
| 08 | Whether `+ New person named 98861 40023` should appear when the typed text is all digits. Recommend hiding it. |
| 09 | `Nobody by that name is holding anything.` also covers a search by mobile, where the wording is slightly off. |
| 09 | Whether `Who is handing it back` should also be a picker. |
| 10 | Where returns are listed — this spec puts them in a `Came back` section at the foot of the "Out with people" tab, because the four approved tabs give them no home. |
| 10 | Supplier row ordering is alphabetical; the mockup shows Sharma before Gupta. |
| 11 | A reason is required on delete. Confirm, or make it optional. |
| 11 | Whether a deleted entry can be un-deleted. Recommend not. |
| 11 | Whether `Received by`, `Person incharge` and `Taken back by` should be correctable. Recommend not. |
| 11 | The five refusal sentences and the delete prompt need a plain-language pass. |
| 12 | The main lines, `sm` lines and empty-state sentences need the same plain-language pass. |

### Wording — reviewed and applied

Items 22–26 went to `plain-language-reviewer`, which read 139 strings and flagged 29.
The rewrites are already applied throughout specs 02 and 04–10, including every test
assertion that quoted an old string. The substantive changes:

- **The refusals no longer break the no-pluralisation rule.** `Only <n> <productWord>
  are on hand. Issue <n> or fewer.` reads `Only 1 chairs are on hand` at a quantity of
  one. Replaced with `You have 890 chairs. You cannot give out more than 890.`, which
  carries the product's own plural and never inflects a bare number. Same for the
  over-return sentence.
- **The `.bak` banner no longer asks for recall.** It said "check the newest entries
  are all here" — addressed to someone who may have just walked in and never saw them.
  It now states the timestamp it recovered to and says what to re-enter.
- **The Suppliers alarm is no longer a red banner.** Something is out with somebody for
  the whole of a gathering, so it would be permanently on and would teach the reader to
  skip red — including the recovery banner that matters.
- **The noun travels with the number** everywhere on the returning screen. `What
  happened to the 5?` became `What happened to the 5 chairs?`.
- **One name per person per screen.** The returning screen called the taker `Ravi` in
  one sentence and `Ravi Menon` in two others.
- **`Taken from someone before`** read as taken *from* the person — the opposite of its
  meaning, and exactly backwards for a second-language reader.
- Confirmations moved from `500 chairs added. Chairs now reads 890 on hand.` to
  `Added 500 chairs. Chairs: 890 on hand.`
- Console output leads with the address, because a reader who sees `Store Register is
  running.` on line one looks away before reaching the warning below it.
- Spec 02 and spec 04 disagreed on whether the recovery banner can be dismissed. Spec
  04 governs: it cannot.

**One recommendation rejected.** The reviewer proposed changing
`Purchased for resale — does not come back` to `Bought — we keep it`, reading "resale"
as forbidden money language. It is not: `Purchase/Resale` is a column from the user's
own handwritten book and is the vocabulary his staff already use. The replacement is
also wrong on the facts — resale stock is sold, not kept. The label stands as approved.
