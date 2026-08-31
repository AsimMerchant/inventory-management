package register

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCleanName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Ravi  Menon ", "Ravi Menon"},
		{"Sharma Tent House", "Sharma Tent House"},
		{"Water drums (20L)", "Water drums (20L)"},
		{"Chairs\t", "Chairs"},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := CleanName(c.in); got != c.want {
			t.Errorf("CleanName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFoldKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Chairs", "chairs"},
		{"chairs", "chairs"},
		{"CHAIRS", "chairs"},
		{"  chairs  ", "chairs"},
		{"Chairs\t", "chairs"},
		{"Ravi  Menon", "ravi menon"},
		{" ravi menon ", "ravi menon"},
		{"Water drums (20L)", "water drums (20l)"},
	}
	for _, c := range cases {
		if got := FoldKey(c.in); got != c.want {
			t.Errorf("FoldKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMobileKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"98861 40023", "9886140023"},
		{"9886140023", "9886140023"},
		{"98861-40023", "9886140023"},
		{" 98861 40023 ", "9886140023"},
		{"", ""},
		{"   ", ""},
		{"+91 98861 40023", "919886140023"},
	}
	for _, c := range cases {
		if got := MobileKey(c.in); got != c.want {
			t.Errorf("MobileKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReturnQuantitySumsAllocations(t *testing.T) {
	r := WalkthroughT3()
	if got := r.Returns[0].Quantity(); got != 45 {
		t.Errorf("RET-0001 Quantity() = %d, want 45", got)
	}
	if got := (Return{}).Quantity(); got != 0 {
		t.Errorf("empty return Quantity() = %d, want 0", got)
	}
}

func TestFixtureRoundTripsThroughJSON(t *testing.T) {
	orig := WalkthroughT0()
	b, err := json.MarshalIndent(orig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var back Register
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orig, &back) {
		t.Errorf("round trip changed the register")
	}
	for _, want := range []string{
		`"receivedOn": "2026-09-01"`,
		`"issuedAt": "2026-09-03T09:40:00+05:30"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded fixture missing %s", want)
		}
	}
}

func TestEmptyRegisterEncodesEmptyArrays(t *testing.T) {
	r := &Register{
		SchemaVersion: 1,
		Products:      []Product{},
		Staff:         []Staff{},
		Inwards:       []Inward{},
		Issues:        []Issue{},
		Returns:       []Return{},
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"products": []`, `"staff": []`, `"inwards": []`, `"issues": []`, `"returns": []`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded empty register missing %s\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("encoded empty register contains null:\n%s", b)
	}
}

func TestUntouchedRecordsCarryNoAuditKeys(t *testing.T) {
	b, err := json.MarshalIndent(WalkthroughT0(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{`"changes"`, `"deleted"`} {
		if strings.Contains(string(b), unwanted) {
			t.Errorf("encoded fixture contains %s", unwanted)
		}
	}
}

func TestCorrectedRecordRoundTrips(t *testing.T) {
	r := WalkthroughT1()
	r.Inwards[6].Changes = []Change{{
		At: MustTime("2026-09-03T10:45:00+05:30"), By: "Suresh Kumar",
		Field: "quantity", Label: "How many", From: "500", To: "50",
	}}
	r.Inwards[1].Deleted = &Deletion{
		At: MustTime("2026-09-03T10:46:00+05:30"), By: "Suresh Kumar",
		Reason: "Entered twice by mistake.",
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var back Register
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, &back) {
		t.Errorf("corrected register did not survive a round trip")
	}
	for _, want := range []string{`"from": "500"`, `"reason": "Entered twice by mistake."`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded register missing %s", want)
		}
	}
}

func TestMustTimePanicsOnRubbish(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustTime did not panic on rubbish")
		}
	}()
	_ = MustTime("not a time")
}

func TestMustTimeMatchesTheFixtureZone(t *testing.T) {
	want := time.Date(2026, time.September, 3, 9, 40, 0, 0, IST)
	if got := MustTime("2026-09-03T09:40:00+05:30"); !got.Equal(want) {
		t.Errorf("MustTime = %v, want %v", got, want)
	}
}
