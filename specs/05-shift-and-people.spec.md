# Spec: Who Is On Duty

## Objective

One question, once, at the start of a shift: whose name goes on everything entered
from now on. Tapping a name is the whole of it — no password, because this is a store
desk and the point is attribution, not security. Afterwards "Person incharge" fills
itself in and can never be blank or misspelt.

## Context

- Owns `internal/web/shift.go`, `internal/web/templates/shift.html` and their tests.
- Depends on `01-data-model.spec.md` (`Staff`, `Register.OnDutyStaffID`,
  `ShiftStartedAt`) and `04-server-and-shell.spec.md` (the guard that sends every
  other route here when no shift is running).

## Contract

### `GET /shift`

Renders the walkthrough's arrival screen, at `max-width:26rem`, with no tabs:

- Heading `Who is at the desk?`
- Sub-heading `Tap your name to begin.`
- One tappable option per `Staff` entry, in the order they appear in the register,
  each reading `<Name> · <Mobile>` — `Suresh Kumar · 98450 22117`. The currently
  on-duty person, if any, is pre-selected (`opt on`).
- Buttons: `Start shift` (primary) and `+ Add a person` (ghost).
- `+ Add a person` reveals an inline block with two fields, `Name` and `Mobile`, and a
  `Save person` button posting to `/shift/person`. It is a `<details>` element, so it
  works with JavaScript switched off.

The selection is a radio group named `staffId` whose values are staff IDs; the person
at the desk sees names, never an ID.

### `POST /shift/start`

Form field `staffId`.

- Unknown or missing: re-render `GET /shift` with a `banner bad` reading
  `Tap your name first.` and status 200.
- Valid: `Update` the register setting `OnDutyStaffID` and `ShiftStartedAt = now`,
  then 303 to `/stock`.

### `POST /shift/person`

Form fields `name`, `mobile`.

- `CleanName(name)` empty: re-render with `banner bad` `Type the person's name.`
- A staff member already exists with the same `FoldKey(name)`: re-render with
  `banner bad` `<Name> is already on the list.`
- Otherwise append `Staff{ID: NextID("STF"), Name: CleanName(name), Mobile: cleaned
  mobile}`, save, and re-render `GET /shift` with the new person pre-selected and a
  `banner ok` reading `<Name> added.`

Mobile cleaning: trim, collapse internal whitespace. Digits are not validated and the
field may be left blank — the walkthrough shows mobiles in the form `98450 22117` but
never says they are required.

### Seeding a fresh register

When `store.Open` reports `Source == Fresh`, the register is created with **no** staff
and no products. `GET /shift` then shows no options and the sentence
`Nobody is on the list yet. Add the first person.` with the add-a-person block already
open. The walkthrough's three names are fixture data, not seed data — a real venue's
staff are not Suresh, Anita and Imran.

### A shift goes stale overnight

`OnDutyStaffID` persists in the data file, so closing and reopening the program during
the same day keeps the shift. It must **not** survive into a new day: the laptop is
shut at night and opened the next morning by whoever arrives first, and a stale shift
would stamp their entries with yesterday's name — the exact attribution failure this
whole screen exists to prevent.

Rule: the shift is live only while `ShiftStartedAt` falls on the same calendar day as
`now`, in the server's local timezone. `onDuty` returns `false` otherwise, which sends
every route to `GET /shift` through the existing guard in
`04-server-and-shell.spec.md`. Nothing is cleared from the file and no record is
rewritten — a stale `OnDutyStaffID` is simply ignored, and the next `POST /shift/start`
overwrites it.

The shift screen after a stale shift is the ordinary one, with the previous person
pre-selected as a convenience. It carries no banner: a new day starting on the
who-is-at-the-desk screen is normal, not an error.

Required tests for this rule:

`TestShiftLivesWithinTheSameDay` — `ShiftStartedAt` at 09:40 on 3 September, clock at
23:58 the same day: `onDuty` returns `Suresh Kumar, true` and `GET /stock` returns 200.

`TestShiftIsStaleOnANewDay` — same `ShiftStartedAt`, clock at 00:02 on 4 September:
`onDuty` returns `false`, `GET /stock` redirects 303 to `/shift`, the saved register
still holds `OnDutyStaffID == "STF-0001"`, and the shift page has `STF-0001`
pre-selected.

`TestStaleShiftIsIgnoredNotCleared` — after the redirect above, no save occurred:
the file's modification time and byte content are unchanged.

### What "on duty" feeds

Every flow spec reads the on-duty person through one helper:

```go
func (s *Server) onDuty(r *register.Register) (register.Staff, bool)
```

- Inward: `ReceivedBy` and `RecordedBy` default to the on-duty name.
- Issue: `PersonInchargeName` and `PersonInchargeMobile` are the on-duty name and
  mobile, rendered read-only as `Anita Rao (you)` and `99001 34562`.
- Return: `TakenBackBy` is the on-duty name, rendered read-only as
  `Imran Sheikh (you)`.

These are not editable on the entry screens. Changing who is on duty means going back
to `/shift`, reachable from the chrome bar by clicking the on-duty name.

## Files to create or modify

- `/home/asim/Projects/inventory-management/internal/web/shift.go`
- `/home/asim/Projects/inventory-management/internal/web/templates/shift.html`
- `/home/asim/Projects/inventory-management/internal/web/shift_test.go`

## Required tests

`TestShiftScreenListsEveryone` — over `WalkthroughT0()`, `GET /shift` returns 200 and
the body contains `Who is at the desk?`, `Tap your name to begin.`,
`Suresh Kumar`, `98450 22117`, `Anita Rao`, `99001 34562`, `Imran Sheikh`,
`90080 77213`, `Start shift` and `+ Add a person`.

`TestStartShiftSetsOnDuty` — `POST /shift/start` with `staffId=STF-0002` returns 303 to
`/stock`; the saved register has `OnDutyStaffID == "STF-0002"` and `ShiftStartedAt`
equal to the injected clock. `GET /stock` then shows `Anita Rao · on duty`.

`TestStartShiftWithNoSelection` — `POST /shift/start` with no `staffId` returns 200
containing `Tap your name first.`; `OnDutyStaffID` is unchanged and nothing was
written to disk.

`TestStartShiftWithUnknownStaff` — `staffId=STF-9999` behaves as above.

`TestAddPerson` — `POST /shift/person` with `name=Imran  Sheikh Jr &mobile=90080 77214`
appends `STF-0004` named `Imran Sheikh Jr` (whitespace collapsed), returns 200
containing `Imran Sheikh Jr added.`, and the file on disk has four staff.

`TestAddDuplicatePersonRefused` — `name=  anita   rao ` against `WalkthroughT0()`
returns 200 containing `Anita Rao is already on the list.` and the register still has
three staff.

`TestAddPersonWithBlankName` — `name=   ` returns 200 containing
`Type the person's name.` and three staff.

`TestFreshRegisterPromptsForFirstPerson` — a `Fresh` register: `GET /shift` contains
`Nobody is on the list yet. Add the first person.` and does not contain `Suresh Kumar`.

`TestEveryFlowRouteBlockedWithoutShift` — with `OnDutyStaffID` empty,
`GET /inward/new`, `GET /issue/new`, `GET /return/new`, `GET /suppliers` each return
303 to `/shift`.

`TestOnDutyNameStampsAnEntry` — with Anita Rao on duty, post a valid issue of 10 chairs
to Ravi Menon; the saved `Issue` has `PersonInchargeName == "Anita Rao"` and
`PersonInchargeMobile == "99001 34562"`, and neither field was accepted from the form
even when the request body supplies `personInchargeName=Somebody Else`.

`TestNoPasswordFieldAnywhere` — the rendered `/shift` body contains no
`type="password"`.

## Acceptance criteria

1. `go test ./internal/web/ -run TestShift -count=1` and `-run 'TestStart|TestAdd|TestFresh|TestEveryFlow|TestOnDuty|TestNoPassword'` pass.
2. **Nobody logs in.** `grep -rniE 'password|passcode|\bpin\b|login|log in|sign in|authenticate|permission|role' --include=*.go --include=*.html --include=*.js . `
   returns nothing across the whole tree. The shift screen puts a name on entries; that
   is the entirety of the security model.
3. `grep -rn --include=*.go --exclude=*_test.go 'Suresh Kumar' internal/web/ main.go`
   returns nothing — the walkthrough's names exist only in
   `internal/register/fixture.go` and in test files, never in shipped handler or
   template code. Also `grep -rn 'Suresh Kumar' internal/web/templates/` returns
   nothing.
4. `grep -n 'STF-' internal/web/templates/shift.html` returns nothing — IDs are values,
   never displayed text.

## Verification commands

```
cd /home/asim/Projects/inventory-management
go test ./internal/web/ -count=1 -v
grep -rn --include=*.go --exclude=*_test.go 'Suresh Kumar' internal/web/ main.go   # nothing
```

## Open

1. **Ending a shift.** The walkthrough shows starting one and never ending one.
   Recommend: clicking the on-duty name in the chrome bar returns to `/shift`; there is
   no "end shift" button and no idle timeout. Confirm.
2. **Removing or renaming a person on the list.** Not shown. Recommend: not built —
   entries already made carry the name as text, so a delete would not rewrite history.
3. **Is mobile required when adding a person?** Not stated. Recommend optional.
4. **Seeding.** Confirmed here as empty. If the venue's three names should be
   pre-loaded into the shipped binary, say so and this becomes a one-line change.
