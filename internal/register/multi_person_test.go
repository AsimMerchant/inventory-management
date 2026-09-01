package register

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func jointTestRegister() *Register {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.FixedZone("IST", 19800))
	return &Register{SchemaVersion: 1,
		Products: []Product{{ID: "PRD-0001", Name: "Chairs"}},
		Inwards:  []Inward{{ID: "INW-0001", ProductID: "PRD-0001", Quantity: 100, RecordedAt: now}},
	}
}

func TestRecipientsOfLegacyIssue(t *testing.T) {
	is := Issue{TakerName: "Ravi Menon", TakerDepartment: "Catering", TakerMobile: "98861 40023"}
	recipients := RecipientsOf(is)
	if len(recipients) != 1 || recipients[0].Name != "Ravi Menon" {
		t.Fatalf("recipients = %#v", recipients)
	}
	recipients[0].Name = "changed"
	if is.TakerName != "Ravi Menon" {
		t.Fatal("RecipientsOf did not return a copy")
	}
	b, err := json.Marshal(is)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "additionalTakers") {
		t.Fatalf("legacy JSON gained field: %s", b)
	}
}

func TestRecipientLabel(t *testing.T) {
	names := []string{"Ravi Menon", "Amit Sharma", "Suresh Patel", "Meera Pillai"}
	wants := []string{"Ravi Menon", "Ravi Menon and Amit Sharma", "Ravi Menon, Amit Sharma and Suresh Patel", "Ravi Menon, Amit Sharma, Suresh Patel and Meera Pillai"}
	for n, want := range wants {
		is := Issue{TakerName: names[0]}
		for _, name := range names[1 : n+1] {
			is.AdditionalTakers = append(is.AdditionalTakers, IssueRecipient{Name: name})
		}
		if got := RecipientLabel(is); got != want {
			t.Errorf("%d names: got %q want %q", n+1, got, want)
		}
	}
}

func TestJointIssueCountsQuantityOnce(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = append(r.Issues, Issue{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", IssuedAt: now, AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma"}, {Name: "Suresh Patel"}}})
	if got := OutWithPeople(r, "PRD-0001"); got != 30 {
		t.Fatalf("out = %d", got)
	}
	if got := OnHand(r, "PRD-0001"); got != 70 {
		t.Fatalf("on hand = %d", got)
	}
	r.Issues[0].AdditionalTakers = append(r.Issues[0].AdditionalTakers, IssueRecipient{Name: "Meera Pillai"})
	if got := OutWithPeople(r, "PRD-0001"); got != 30 {
		t.Fatalf("out after recipient = %d", got)
	}
}

func TestJointHoldingsSeparateRaviSoloFromGroup(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 3, TakerName: "Ravi Menon", IssuedAt: now},
		{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", IssuedAt: now.Add(time.Minute), AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma", Department: "Setup", Mobile: "97740 11298"}, {Name: "Suresh Patel"}}},
	}
	holdings := JointHoldings(r)
	if len(holdings) != 2 {
		t.Fatalf("holdings = %#v", holdings)
	}
	total := 0
	for _, h := range holdings {
		total += h.TotalOut
	}
	if total != 33 {
		t.Fatalf("total = %d", total)
	}
}

func TestFindJointHoldingThroughEveryMember(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", IssuedAt: now, AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma", Department: "Setup", Mobile: "97740 11298"}, {Name: "Suresh Patel"}}}}
	for _, q := range []string{"Ravi", "Amit", "Suresh", "Setup", "9774011298"} {
		got := FindJointHoldings(r, q)
		if len(got) != 1 || got[0].AnchorIssueID != "ISS-0001" {
			t.Errorf("%q found %#v", q, got)
		}
	}
	if got := FindJointHoldings(r, "Meera"); len(got) != 0 {
		t.Fatalf("Meera found %#v", got)
	}
}

func TestPlanReturnCannotCrossHoldingBoundary(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 3, TakerName: "Ravi Menon", IssuedAt: now},
		{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", IssuedAt: now.Add(time.Minute), AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma"}, {Name: "Suresh Patel"}}},
	}
	if got := PlanReturn(r, []string{"ISS-0001"}, 3).Allocations; len(got) != 1 || got[0].IssueID != "ISS-0001" {
		t.Fatalf("solo plan = %#v", got)
	}
	if got := PlanReturn(r, []string{"ISS-0002"}, 20).Allocations; len(got) != 1 || got[0].IssueID != "ISS-0002" {
		t.Fatalf("group plan = %#v", got)
	}
}

func TestRepeatedJointRecipientsStayIssueSpecific(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	group := []IssueRecipient{{Name: "Amit Sharma"}, {Name: "Suresh Patel"}}
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 10, TakerName: "Ravi Menon", IssuedAt: now, AdditionalTakers: group},
		{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 20, TakerName: "Ravi Menon", IssuedAt: now.Add(time.Minute), AdditionalTakers: group},
		{ID: "ISS-0003", ProductID: "PRD-0001", Quantity: 2, TakerName: "Ravi Menon", IssuedAt: now.Add(2 * time.Minute)},
		{ID: "ISS-0004", ProductID: "PRD-0001", Quantity: 1, TakerName: "Ravi Menon", IssuedAt: now.Add(3 * time.Minute)},
	}
	holdings := JointHoldings(r)
	if len(holdings) != 3 {
		t.Fatalf("holdings = %d, want two group plus one solo", len(holdings))
	}
	for _, holding := range holdings {
		if len(holding.Recipients) == 1 && len(holding.Lines) != 2 {
			t.Fatalf("solo lines = %#v", holding.Lines)
		}
	}
}

func TestPeopleHoldingCountsMembersNotQuantity(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 3, TakerName: "Ravi Menon", IssuedAt: now},
		{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", IssuedAt: now.Add(time.Minute), AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma"}, {Name: "Suresh Patel"}}},
	}
	people := PeopleHolding(r)
	if len(people) != 3 {
		t.Fatalf("people = %#v", people)
	}
	if tiles := TileCounts(r, now); tiles.PeopleHolding != 3 || tiles.OutRightNow != 33 {
		t.Fatalf("tiles = %#v", tiles)
	}
}

func TestPeopleHoldingKeepsLatestDetailsFromSettledIssue(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 3, TakerName: "Ravi Menon", TakerDepartment: "Earlier", TakerMobile: "98861 40023", IssuedAt: now},
		{ID: "ISS-0002", ProductID: "PRD-0001", Quantity: 2, TakerName: "RAVI MENON", TakerDepartment: "Latest", TakerMobile: "98861 40023", IssuedAt: now.Add(time.Minute)},
	}
	r.Returns = []Return{{ID: "RET-0001", ProductID: "PRD-0001", Allocations: []Allocation{{IssueID: "ISS-0002", Quantity: 2}}}}
	people := PeopleHolding(r)
	if len(people) != 1 || people[0].Name != "RAVI MENON" || people[0].Department != "Latest" || people[0].TotalOut != 3 {
		t.Fatalf("people = %#v", people)
	}
}

func TestLogJointIssueIsOneSearchableRow(t *testing.T) {
	r := jointTestRegister()
	now := r.Inwards[0].RecordedAt.Add(time.Hour)
	r.Issues = []Issue{{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 30, TakerName: "Ravi Menon", TakerDepartment: "Catering", IssuedAt: now, RecordedAt: now, AdditionalTakers: []IssueRecipient{{Name: "Amit Sharma", Department: "Setup", Mobile: "97740 11298"}, {Name: "Suresh Patel"}}}}
	entries := LogEntries(r)
	for _, q := range []string{"Ravi", "Amit", "Suresh", "Setup", "9774011298"} {
		got := FilterLog(r, entries, LogFilter{Query: q}, time.Local)
		if len(got) != 1 || got[0].RecordID != "ISS-0001" || len(got[0].Recipients) != 3 {
			t.Fatalf("query %q = %#v", q, got)
		}
	}
}
