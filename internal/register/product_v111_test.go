package register

import (
	"reflect"
	"testing"
	"time"
)

func TestRenameProductKeepsIdentityAndStock(t *testing.T) {
	r := WalkthroughT3()
	findRow := func() StockRow {
		for _, row := range StockRows(r) {
			if row.ProductID == "PRD-0001" {
				return row
			}
		}
		return StockRow{}
	}
	before := findRow()
	p := r.Products[0]
	at := time.Date(2026, 9, 3, 19, 0, 0, 0, IST)
	if err := RenameProduct(r, p.ID, "  Folding   chairs ", "Anita Rao", at); err != nil {
		t.Fatal(err)
	}
	got, ok := ProductByID(r, p.ID)
	if !ok {
		t.Fatal("renamed product is not live")
	}
	if got.ID != p.ID || got.CreatedAt != p.CreatedAt || got.CreatedBy != p.CreatedBy || got.Name != "Folding chairs" {
		t.Fatalf("product = %#v", got)
	}
	after := findRow()
	if before.CameIn != after.CameIn || before.Out != after.Out || before.OnHand != after.OnHand {
		t.Fatalf("stock changed: %#v -> %#v", before, after)
	}
	want := Change{At: at, By: "Anita Rao", Field: "productName", Label: "Product name", From: "Chairs", To: "Folding chairs"}
	if len(got.Changes) != 1 || got.Changes[0] != want {
		t.Fatalf("changes = %#v", got.Changes)
	}
}

func TestProductDeletionImpactAtT3(t *testing.T) {
	r := WalkthroughT3()
	got, ok := ProductDeletionImpact(r, "PRD-0001")
	if !ok {
		t.Fatal("no impact")
	}
	if got.InwardEntries != 3 || got.IssueEntries != 4 || got.ReturnEntries != 1 || got.CurrentlyOut != 275 || got.Version == "" {
		t.Fatalf("impact = %#v", got)
	}
	reverseProducts := func() {
		for i, j := 0, len(r.Products)-1; i < j; i, j = i+1, j-1 {
			r.Products[i], r.Products[j] = r.Products[j], r.Products[i]
		}
	}
	reverseInwards := func() {
		for i, j := 0, len(r.Inwards)-1; i < j; i, j = i+1, j-1 {
			r.Inwards[i], r.Inwards[j] = r.Inwards[j], r.Inwards[i]
		}
	}
	reverseIssues := func() {
		for i, j := 0, len(r.Issues)-1; i < j; i, j = i+1, j-1 {
			r.Issues[i], r.Issues[j] = r.Issues[j], r.Issues[i]
		}
	}
	reverseReturns := func() {
		for i, j := 0, len(r.Returns)-1; i < j; i, j = i+1, j-1 {
			r.Returns[i], r.Returns[j] = r.Returns[j], r.Returns[i]
		}
	}
	reverseProducts()
	reverseInwards()
	reverseIssues()
	reverseReturns()
	again, _ := ProductDeletionImpact(r, "PRD-0001")
	if got.Version != again.Version {
		t.Fatalf("version changed with order: %s != %s", got.Version, again.Version)
	}
}

func TestDeleteProductCascadeTombstonesEveryRelatedRecord(t *testing.T) {
	r := WalkthroughT3()
	at := time.Date(2026, 9, 3, 19, 1, 0, 0, IST)
	if err := DeleteProductCascade(r, "PRD-0001", "Anita Rao", at, " Goods   never arrived. "); err != nil {
		t.Fatal(err)
	}
	want := &Deletion{At: at, By: "Anita Rao", Reason: "Goods never arrived."}
	if !reflect.DeepEqual(r.Products[0].Deleted, want) {
		t.Fatalf("product deletion = %#v", r.Products[0].Deleted)
	}
	for _, in := range r.Inwards {
		if in.ProductID == "PRD-0001" && !reflect.DeepEqual(in.Deleted, want) {
			t.Fatalf("inward %s not deleted", in.ID)
		}
	}
	for _, is := range r.Issues {
		if is.ProductID == "PRD-0001" && is.Deleted == nil {
			t.Fatalf("issue %s not deleted", is.ID)
		}
	}
	for _, re := range r.Returns {
		if re.ProductID == "PRD-0001" && re.Deleted == nil {
			t.Fatalf("return %s not deleted", re.ID)
		}
	}
}

func TestCascadeMakesNoStockImpossible(t *testing.T) {
	r := WalkthroughT3()
	other := map[string][4]int{}
	for _, p := range r.Products {
		if p.ID != "PRD-0001" {
			other[p.ID] = [4]int{CameIn(r, p.ID), Returned(r, p.ID), OutWithPeople(r, p.ID), OnHand(r, p.ID)}
		}
	}
	if err := DeleteProductCascade(r, "PRD-0001", "Anita Rao", time.Now(), "Wrong product"); err != nil {
		t.Fatal(err)
	}
	if got := []int{CameIn(r, "PRD-0001"), Returned(r, "PRD-0001"), OutWithPeople(r, "PRD-0001"), OnHand(r, "PRD-0001")}; !reflect.DeepEqual(got, []int{0, 0, 0, 0}) {
		t.Fatalf("deleted stock = %v", got)
	}
	if problems := Validate(r); len(problems) != 0 {
		t.Fatalf("problems = %#v", problems)
	}
	for id, want := range other {
		got := [4]int{CameIn(r, id), Returned(r, id), OutWithPeople(r, id), OnHand(r, id)}
		if got != want {
			t.Fatalf("%s changed: %v -> %v", id, want, got)
		}
	}
}

func TestCascadePreservesEarlierTombstonesAndCorrections(t *testing.T) {
	r := WalkthroughT3()
	old := Deletion{At: r.Issues[0].IssuedAt, By: "Suresh", Reason: "Earlier"}
	r.Issues[0].Deleted = &old
	r.Issues[0].Changes = []Change{{Field: "quantity", From: "10", To: "9"}}
	allocations := append([]Allocation(nil), r.Returns[0].Allocations...)
	if err := DeleteProductCascade(r, "PRD-0001", "Anita", time.Now(), "Wrong product"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Issues[0].Deleted, &old) || len(r.Issues[0].Changes) != 1 || !reflect.DeepEqual(r.Returns[0].Allocations, allocations) {
		t.Fatal("cascade changed earlier audit data")
	}
}

func challanFixture() *Register {
	r := &Register{SchemaVersion: SchemaVersion, Products: []Product{{ID: "PRD-0001", Name: "Chairs"}, {ID: "PRD-0002", Name: "Cables"}}, Inwards: []Inward{{ID: "INW-0001", ProductID: "PRD-0001", Quantity: 100}, {ID: "INW-0002", ProductID: "PRD-0002", Quantity: 100}}}
	base := time.Date(2026, 9, 3, 13, 0, 0, 0, IST)
	r.Issues = []Issue{
		{ID: "ISS-0001", ProductID: "PRD-0001", Quantity: 20, ChallanNo: "452", TakerName: "Ravi", TakerMobile: "1", IssuedAt: base},
		{ID: "ISS-0002", ProductID: "PRD-0002", Quantity: 10, ChallanNo: "CH-452", TakerName: "Ravi", TakerMobile: "1", AdditionalTakers: []IssueRecipient{{Name: "Amit", Mobile: "2"}}, IssuedAt: base.Add(time.Minute)},
		{ID: "ISS-0003", ProductID: "PRD-0001", Quantity: 5, ChallanNo: "452-A", TakerName: "Meera", TakerMobile: "3", IssuedAt: base.Add(2 * time.Minute)},
		{ID: "ISS-0004", ProductID: "PRD-0001", Quantity: 4, ChallanNo: "CH-999", TakerName: "Meera", TakerMobile: "3", IssuedAt: base.Add(3 * time.Minute)},
	}
	return r
}

func TestFindOutstandingByChallanPartialCaseInsensitive(t *testing.T) {
	r := challanFixture()
	got := FindOutstandingByChallan(r, "45")
	want := []string{"ISS-0001", "ISS-0003", "ISS-0002"}
	var ids []string
	for _, m := range got {
		ids = append(ids, m.IssueID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v", ids)
	}
	got = FindOutstandingByChallan(r, "ch-452")
	if len(got) != 1 || got[0].IssueID != "ISS-0002" || got[0].Outstanding != 10 || got[0].RecipientLabel != "Ravi and Amit" {
		t.Fatalf("match = %#v", got)
	}
}

func TestChallanMatchCarriesSafeHoldingAnchor(t *testing.T) {
	r := challanFixture()
	r.Issues = append(r.Issues, Issue{ID: "ISS-0005", ProductID: "PRD-0001", Quantity: 3, ChallanNo: "452", TakerName: "Ravi", TakerMobile: "1", IssuedAt: r.Issues[0].IssuedAt.Add(time.Second)})
	got := FindOutstandingByChallan(r, "452")
	var solo, joint ChallanMatch
	for _, m := range got {
		if m.IssueID == "ISS-0005" {
			solo = m
		}
		if m.IssueID == "ISS-0002" {
			joint = m
		}
	}
	if solo.HoldingIssueID != "ISS-0001" || joint.HoldingIssueID != "ISS-0002" {
		t.Fatalf("anchors solo=%s joint=%s", solo.HoldingIssueID, joint.HoldingIssueID)
	}
	joint.Recipients[0].Name = "changed"
	if r.Issues[1].TakerName == "changed" {
		t.Fatal("recipients aliased")
	}
}

func TestFindOutstandingByChallanReturnsEveryProductAndGroup(t *testing.T) {
	r := challanFixture()
	got := FindOutstandingByChallan(r, "452")
	if len(got) != 3 {
		t.Fatalf("matches=%d", len(got))
	}
	quantities := map[string]int{}
	for _, m := range got {
		quantities[m.IssueID] = m.Outstanding
	}
	for id, want := range map[string]int{"ISS-0001": 20, "ISS-0002": 10, "ISS-0003": 5} {
		if quantities[id] != want {
			t.Fatalf("%s=%d", id, quantities[id])
		}
	}
}

func TestFindOutstandingByChallanSkipsSettledAndDeleted(t *testing.T) {
	r := challanFixture()
	r.Returns = []Return{{ID: "RET-1", ProductID: "PRD-0001", Allocations: []Allocation{{IssueID: "ISS-0001", Quantity: 20}}}}
	r.Issues[2].Deleted = &Deletion{Reason: "gone"}
	r.Products[1].Deleted = &Deletion{Reason: "gone"}
	if got := FindOutstandingByChallan(r, "45"); len(got) != 0 {
		t.Fatalf("matches=%#v", got)
	}
}

func TestLogCarriesInwardAndIssueChallans(t *testing.T) {
	r := challanFixture()
	r.Inwards[0].ChallanNo = "STH/4471"
	r.Issues[0].ChallanNo = "CH-452"
	entries := LogEntries(r)
	foundIn, foundOut := false, false
	for _, e := range entries {
		if e.RecordID == "INW-0001" && e.Kind == LogCameIn {
			foundIn = e.ChallanNo == "STH/4471"
		}
		if e.RecordID == "ISS-0001" && e.Kind == LogWentOut {
			foundOut = e.ChallanNo == "CH-452"
		}
	}
	if !foundIn || !foundOut {
		t.Fatalf("challans absent: in=%v out=%v", foundIn, foundOut)
	}
}

func TestLogFilterByPartialChallan(t *testing.T) {
	r := challanFixture()
	entries := FilterLog(r, LogEntries(r), LogFilter{ChallanQuery: "45"}, IST)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
}
