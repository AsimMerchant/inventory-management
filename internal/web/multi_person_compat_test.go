package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
	"storeregister/internal/store"
)

func TestOldStakeholderFileRoundTrips(t *testing.T) {
	source := filepath.Join("..", "store", "testdata", "walkthrough-t0.json")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), store.FileName)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	st, _, err := store.Open(path)
	if err != nil {
		t.Fatalf("opening pre-feature file: %v", err)
	}
	var before register.Register
	st.Read(func(reg *register.Register) { before = *reg })
	beforeOut := register.OutWithPeople(&before, "PRD-0001")
	beforeOnHand := register.OnHand(&before, "PRD-0001")
	beforeAllocations := make([][]register.Allocation, len(before.Returns))
	for i := range before.Returns {
		beforeAllocations[i] = append([]register.Allocation(nil), before.Returns[i].Allocations...)
	}

	srv := NewServer(st, "", func() time.Time { return tenAM })
	ts := httptest.NewServer(srv)
	e := &env{Server: ts, path: path, st: st, srv: srv, t: t}
	for _, route := range []string{"/stock", "/out", "/return/new", "/log?day=all"} {
		resp, _ := e.get(route)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", route, resp.StatusCode)
		}
	}

	form := url.Values{
		"productId":       {"PRD-0001"},
		"quantity":        {"1"},
		"takerName":       {"Meera Pillai"},
		"takerDepartment": {"Hospitality"},
		"takerMobile":     {"95550 11223"},
		"issuedAt":        {"2026-09-03T10:00"},
	}
	resp, body := e.post("/issue/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("solo issue = %d: %s", resp.StatusCode, body)
	}
	ts.Close()

	reopened, _, err := store.Open(path)
	if err != nil {
		t.Fatalf("reopening saved file: %v", err)
	}
	var after register.Register
	reopened.Read(func(reg *register.Register) { after = *reg })
	if register.OutWithPeople(&after, "PRD-0001") != beforeOut+1 || register.OnHand(&after, "PRD-0001") != beforeOnHand-1 {
		t.Fatalf("old totals changed incorrectly: before out/onhand %d/%d, after %d/%d", beforeOut, beforeOnHand, register.OutWithPeople(&after, "PRD-0001"), register.OnHand(&after, "PRD-0001"))
	}
	for i := range beforeAllocations {
		if !reflect.DeepEqual(after.Returns[i].Allocations, beforeAllocations[i]) {
			t.Fatalf("return %d allocations changed", i)
		}
	}
	if len(register.Validate(&after)) != 0 {
		t.Fatalf("reopened register is invalid: %#v", register.Validate(&after))
	}
	for _, issue := range after.Issues {
		if len(issue.AdditionalTakers) != 0 {
			t.Fatalf("solo legacy file gained recipients: %#v", issue)
		}
	}
	savedRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(savedRaw), "additionalTakers") {
		t.Fatal("one-person round trip wrote additionalTakers")
	}
	var decoded register.Register
	if err := json.Unmarshal(savedRaw, &decoded); err != nil {
		t.Fatalf("saved JSON unreadable: %v", err)
	}
}
