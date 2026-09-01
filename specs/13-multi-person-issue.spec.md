# Spec: More Than One Person on an Issue

## Objective

Record one total quantity of one product as handed to one or more named people.
When Ravi Menon, Amit Sharma and Suresh Patel collect 30 chairs together, the
register stores one issue of 30 chairs and shows all three names. It never divides
the 30 between them, counts it three times, or creates a permanent group.

If Ravi later collects 3 chairs alone, his solo 3-chair holding and the joint
30-chair holding remain separate and selectable during a return without asking the
desk to type or remember an issue ID.

## Context

- Recorded stakeholder decision, 1 September 2026: every person who came together
  to collect the quantity must be nameable; the one quantity is held jointly; a
  member may separately collect stock alone; membership belongs only to that issue.
- The approved walkthrough and specs 01, 03 and 08-12 continue to govern every
  one-person issue.
- One issue still has one product, one total quantity, one issue time and one person
  in charge. Multi-product issuing is not added.
- No `Group` type, group ID, group name or reusable group list is stored.

## Contract

### Inputs

#### File format and register API

Add to `internal/register/model.go`:

```go
type IssueRecipient struct {
    Name       string `json:"name"`
    Department string `json:"department"`
    Mobile     string `json:"mobile"`
}
```

Keep every existing `Issue` field and add immediately after `TakerMobile`:

```go
AdditionalTakers []IssueRecipient `json:"additionalTakers,omitempty"`
```

The existing `TakerName`, `TakerDepartment` and `TakerMobile` are recipient 1.
`AdditionalTakers`, in stored order, are recipients 2 onward. They carry no
quantity. `SchemaVersion` remains 1. Old JSON with no new field loads unchanged;
one-person issues omit the new field when saved.

Add:

```go
func RecipientsOf(Issue) []IssueRecipient
func RecipientLabel(Issue) string
```

`RecipientsOf` returns a copy: the legacy recipient followed by
`AdditionalTakers`. `RecipientLabel` preserves order and uses full names:
`Ravi Menon`; `Ravi Menon and Amit Sharma`; `Ravi Menon, Amit Sharma and Suresh
Patel`. Four names use `Ravi Menon, Amit Sharma, Suresh Patel and Meera Pillai`.
There is no Oxford comma.

Clean every recipient name and department with `register.CleanName`; clean mobiles
exactly as the existing `TakerMobile`. Identity remains `PersonOf(Name, Mobile)`.
Do not add duplicate-person warnings or refusals.

#### `GET, POST /issue/new`

The first recipient retains exact fields `takerName`, `takerDepartment` and
`takerMobile`. Directly below it render `+ Add another person`.

Each later recipient renders `Another person taking it`, `Remove this person`, and
aligned repeated fields:

```text
additionalTakerName
additionalTakerDepartment
additionalTakerMobile
```

Below the first added recipient show:
`These people are taking it together. Enter the total quantity only once.` No
recipient row has a quantity field.

`addPerson=1` and `removePerson=<zero-based additional-recipient index>` re-render
at 200, preserve every typed value, and perform no store update. They must work with
JavaScript disabled. JavaScript may add/remove in place but must post the same arrays.
Each person picker fills only its own row's department and mobile.

The one-person button stays `Issue 10 chairs to Ravi`. A completed group button is
`Issue 30 chairs to Ravi, Amit and Suresh`; it uses first names and the same join
rule. Otherwise it reads `Issue`.

The existing spec 08 validation order remains. Immediately after validating the
first name, reject a blank additional name or unequal additional-field array lengths
at 200 with `Type the name of every person taking it.` Preserve all fields and write
nothing. Empty department/mobile remains allowed.

On success append exactly one `Issue`: quantity 30 once, first recipient in legacy
fields, remaining recipients in `AdditionalTakers`. Never divide/multiply quantity
or append one issue per person. Ignore posted person-in-charge fields as before.
Re-run `CheckIssue` inside the single `store.Update` callback.

Redirect to `/stock?saved=<issueID>`. The T1 worked example leaves 860 chairs and
shows `Gave 30 chairs to Ravi Menon, Amit Sharma and Suresh Patel. Chairs: 860 on
hand.` Single-person success wording is unchanged.

#### Holdings and search

Add in `internal/register/arith.go`:

```go
type JointHolding struct {
    AnchorIssueID string
    Recipients    []IssueRecipient
    Label         string
    TotalOut      int
    Lines         []OutstandingLine
}
func JointHoldings(*Register) []JointHolding
func FindJointHoldings(*Register, string) []JointHolding
func JointHoldingForIssue(*Register, string) (JointHolding, bool)
```

- Solo live issues continue to group by `PersonOf` exactly as today; Ravi's two solo
  chair lines can still be returned oldest-first together.
- Each live multi-person issue is its own holding, even if a later issue names the
  same people. Membership is issue-specific.
- Omit settled/deleted lines and zero-total holdings. Lines sort by `IssuedAt`, then
  issue ID. Holdings sort by folded label, earliest line time, then anchor ID.
- A group anchor is that issue ID. A solo anchor is the oldest outstanding issue ID
  for that person. `JointHoldingForIssue` rejects unknown, deleted or settled IDs.
- `FindJointHoldings` applies existing name/department/mobile substring matching to
  every member and returns a matching holding once.

`PeopleHolding`/`FindPeople` remain individual-person APIs for pickers and the tile.
They include every member, keyed by `PersonOf`. The tile counts distinct named
people: Ravi/Amit/Suresh count as three; Ravi's solo issue does not count Ravi twice.
These APIs may describe a member's holdings but must never sum stock.

All stock functions use the one issue quantity only. Recipient count is absent from
`CameIn`, `Returned`, `OutWithPeople`, `OnHand`, `OutstandingOnIssue`, `CheckIssue`,
`CheckReturn` and `Validate`.

#### Returns

The existing return person picker still selects an individual. Once selected,
`GET /return/new` shows every `JointHolding` containing that exact person. For Ravi's
worked example the accessible headings are:

```text
Ravi Menon - holding alone
Ravi Menon, Amit Sharma and Suresh Patel - holding together
```

The group shows one line `30 taken, 0 back` / `30 out`, never one 30-chair line per
member. Searching Ravi, Amit, Suresh or any member's mobile/department finds it.

A selectable holding carries `holdingIssueId=<AnchorIssueID>` plus existing query
values. The user only taps its labelled row. GET and POST re-derive it with
`JointHoldingForIssue`; a stale/unknown/deleted/settled anchor returns 200 with
`That holding has changed. Pick it again from the list.` and writes nothing.

For a group return, hidden `issueIds` contains only that group issue. For a solo
return it contains the existing same-person/product outstanding issue set. Thus a
solo return cannot allocate to the group and a group return cannot allocate to
Ravi's solo issue. Re-derive the holding and run `CheckReturn` inside the same
`store.Update` callback that appends the return.

The selected label and shortfall text use all full names. Five short reads:
`5 chairs missing. Ravi Menon, Amit Sharma and Suresh Patel still have them.` Missing
stock stays jointly against the issue; it is not assigned to the search member,
first member or physical returner. `Who is handing it back` stays separate and
editable, defaulting to the selected searched member.

`Return` JSON is unchanged; allocations already identify the issue. `TakerOf`,
confirmations and correction refusals derive `RecipientLabel` from allocations.

#### Out view, corrections and activity log

`GET /out` renders `JointHoldings`, never duplicated individual stock. The worked
headings are `Ravi Menon - holding alone - 3 out` and `Ravi Menon, Amit Sharma and
Suresh Patel - holding together - 30 out`, with one group issue row.

`entryName` and issue log lines use all names: `30 chairs to Ravi Menon, Amit Sharma
and Suresh Patel`; `30 chairs went out to Ravi Menon, Amit Sharma and Suresh Patel`.
Add to derived `LogEntry`:

```go
Recipients []IssueRecipient
```

Issue original/corrected/deleted entries copy all recipients. Keep existing
`PersonName`, `PersonMobile`, `PersonDepartment` populated from recipient 1 for
compatibility. `matchesQuery`, `PeopleInLog` and `FindPeopleInLog` inspect every
recipient. Any member query returns the one issue row, not one row per member.

The issue correction screen uses the same repeatable controls and may change solo to
joint or joint to solo. Product/person-in-charge remain fixed. A single-person issue
that stays single keeps spec 11's existing `taker`, `department`, `mobile` changes.
When either old or new issue has multiple recipients:

- changed names/count/order append one `Change{Field:"recipients", Label:"Who is
  taking it", From:<old label>, To:<new label>}`;
- changed department/mobile append next one `Change{Field:"recipientDetails",
  Label:"Their details", From:<old details>, To:<new details>}`.

Details format each recipient `Name | Department | Mobile` (preserve empty fields)
and join with `; `. `changePhrase` renders `Changed who took it from <From> to <To>`
and `Changed their details from <From> to <To>`. Corrections/deletions remain one
atomic update, pass `Validate` and the orphan-allocation guard, and never alter a
return allocation when names change.

### Outputs

- One issue stores one quantity and an ordered recipient list of length one or more.
- Stock and log movements count it once; every relevant screen names all recipients.
- Any member search finds the group; solo and joint contexts stay separate.
- All existing one-person data, posts, calculations, wording and allocations remain
  compatible.

### Side effects

- Successful issue/return/correction operations each perform one atomic store save.
- GET, picker, add/remove, and refusal paths never write.
- Save-time stock and selected-holding guards run inside `store.Update`.
- `internal/store` and its atomic `.bak` sequence are unchanged.

## Files to create or modify

- `internal/register/model.go`, `arith.go`, `allocate.go`, `log.go` and their tests.
- `internal/web/issue.go`, `return.go`, `views.go`, `corrections.go`, `log.go`.
- `internal/web/templates/issue.html`, `return.html`, `out.html`, `edit.html`,
  `log.html`.
- `internal/web/static/issue.js`, `person-picker.js`, `return.js` and corresponding
  web tests.
- `specs/00-index.spec.md`.

Do not modify `internal/store` or add a route, dependency or permanent group type.

## Required tests

`TestRecipientsOfLegacyIssue` - T0's `ISS-0003` yields Ravi only and marshals with no
`additionalTakers`.

`TestRecipientLabel` - assert the exact one/two/three/four-name joins above and stored
order.

`TestJointIssueCountsQuantityOnce` - add the 30-chair group at T1; out rises by 30,
on-hand becomes 860, and adding recipients to that issue changes neither number.

`TestJointHoldingsSeparateRaviSoloFromGroup` - 3 chairs to Ravi plus 30 to
Ravi/Amit/Suresh yields exactly two labelled holdings totaling 33.

`TestRepeatedJointRecipientsStayIssueSpecific` - two identical-recipient group issues
stay two holdings; two Ravi solo issues still combine into one solo holding.

`TestFindJointHoldingThroughEveryMember` - Ravi, Amit, Suresh, Amit's department and
`9774011298` each find the same one group; Meera finds none.

`TestPeopleHoldingCountsMembersNotQuantity` - the worked issues yield three distinct
people, Ravi once, and 33 stock out.

`TestPlanReturnCannotCrossHoldingBoundary` - a solo 3 return allocates only solo; a
group 20 return allocates only group and leaves 10 group chairs.

`TestLogJointIssueIsOneSearchableRow` - one quantity-30 log row carries three
recipients; every member query returns that same record once.

`TestIssueSinglePersonRemainsCompatible` - the spec 08 post, JSON, button and success
sentence are unchanged.

`TestIssueAddRemovePersonWithoutJavaScript` - add twice/remove once at 200, preserve
all fields, align arrays, write nothing.

`TestEachRecipientPickerFillsOnlyItsRow` - selecting Amit in row 2 does not change
Ravi or Suresh rows.

`TestIssue30ChairsToThreePeople` - post Ravi/Catering/`98861 40023`,
Amit/Setup/`97740 11298`, Suresh/Logistics/`90080 77001`; assert one issue, quantity
30, ordered additional takers, on-hand 860, 303 and exact success text.

`TestIssueRefusesBlankAdditionalName` - exact banner, 200, retained fields, no write.

`TestJointIssueRechecksStockAtSaveTime` - reduce on-hand to 20 before posting 30;
show existing 20-chair refusal and save nothing.

`TestOutShowsJointQuantityOnce` - exact solo/group headings; one 30-chair group row,
no duplicated stock.

`TestReturnSearchFindsGroupByEveryMember` - Ravi/Amit/Suresh/mobile find the same
anchor; Ravi also sees his separate solo holding.

`TestReturn20FromGroup` and `TestReturn3FromRaviAlone` - allocations never cross and
remaining totals are respectively group 10/solo 3 and group 30/solo 0.

`TestReturnRefusesStaleGroup` - settle/delete after render; exact stale banner, no
new return, valid register.

`TestShortReturnStaysAgainstWholeGroup` - return 25 with required remark/disposition;
all three names remain against 5, not the returner alone.

`TestFixJointRecipientsIsAudited` - Ravi/Amit/Suresh to Ravi/Amit/Meera plus detail
change creates the two exact changes, keeps quantity/allocations, and passes Validate.

`TestLogNamesEveryJointRecipient` - one exact full-name line; each member filter finds
it once.

`TestOldStakeholderFileRoundTrips` - load pre-feature T3 JSON, visit stock/out/return/
log, add one solo issue, save/reopen; old totals and allocations remain valid and old
issues omit `additionalTakers`.

## Acceptance criteria

1. Every required and pre-existing test passes with `-race -count=1`.
2. The worked issue creates one issue, reduces stock by exactly 30, names three people
   and is findable through each.
3. Ravi's solo 3 and joint 30 are separately selectable; returns cannot cross them.
4. Old schema-1 JSON works without migration and one-person JSON omits the new field.
5. No production struct stores a per-recipient quantity or permanent group.
6. `CheckIssue`, selected-holding derivation and `CheckReturn` are present inside the
   relevant `store.Update` callbacks.
7. `internal/store` is unchanged; its recovery tests pass.
8. Standard-library-only Windows amd64 build succeeds with cgo disabled.

## Verification commands

```text
go test ./internal/register/ -run 'TestRecipient|TestJoint|TestPeopleHolding|TestPlanReturn|TestLog' -count=1 -v
go test ./internal/web/ -run 'TestIssue|TestOut|TestReturn|TestFixJoint|TestLogNames|TestOldStakeholder' -count=1 -v
go test ./... -race -count=1
go vet ./...
go test ./internal/store/ -count=1
rg 'CheckIssue' internal/web/issue.go
rg 'JointHoldingForIssue|CheckReturn' internal/web/return.go
rg 'FormValue\("personIncharge' internal/web/issue.go
rg 'type (Group|RecipientGroup)|Groups +\[\]' internal/register internal/store
rg 'net/http' internal/register internal/store
PowerShell -NoProfile -Command "$env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH='amd64'; go build -o $env:TEMP\register.exe ."
```

The last two forbidden-code `rg` commands must print nothing. Positive guard greps
must show calls inside `store.Update` closures.

## Open

1. The approved walkthrough has no multi-person mockup. The new strings in this spec
   are direct extensions of the recorded decision and require the project's
   `plain_language_reviewer` after implementation; any rewrite must amend the
   verbatim tests and this contract together.
2. The stakeholder explicitly requested every member's name. This contract also
   collects the existing department/mobile fields for later members so same-name
   people remain distinguishable and searchable. Field feedback may later simplify
   later members to name-only; it does not block this backward-compatible contract.
