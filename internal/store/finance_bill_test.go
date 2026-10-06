package store

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	"storeregister/internal/register"
)

func TestVersion130SchemaFiveLoadsWithoutWritingAndFirstBillWritesSix(t *testing.T) {
	s, path, key, _ := initializedFinance(t)
	s.mu.Lock()
	s.reg.SchemaVersion = 5
	if err := s.save(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	before, _ := os.ReadFile(path)

	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Read(func(r *register.Register) {
		if r.SchemaVersion != 6 || r.SupplierChallans == nil || len(r.SupplierChallans) != 0 {
			t.Errorf("loaded schema/challans = %d/%#v", r.SchemaVersion, r.SupplierChallans)
		}
	})
	afterOpen, _ := os.ReadFile(path)
	if !bytes.Equal(before, afterOpen) {
		t.Fatal("opening schema 5 wrote the file")
	}
	key, _, err = s.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateFinance(key, func(r *register.Register, f *register.FinanceData) error {
		party, err := register.AddParty(r, "Sharma Tent House")
		if err != nil {
			return err
		}
		purpose, err := register.AddFinanceValue(f, register.FinancePurpose, "Rent", "FAC-0001", financeNow)
		if err != nil {
			return err
		}
		f.Bills = append(f.Bills, register.FinanceBill{ID: f.NextID("BIL"), AmountPaise: 2000000, OccurredAt: financeNow, PartyID: party, PurposeID: purpose, RecordedAt: financeNow, RecordedByID: "FAC-0001"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Contains(raw, []byte(`"schemaVersion": 6`)) {
		t.Fatal("first bill write did not persist schema 6")
	}
	if err := s.ReadFinance(key, func(f *register.FinanceData) {
		if f.Bills == nil || len(f.Bills) != 1 || len(f.Orders) != 0 {
			t.Errorf("bills/orders = %#v/%#v", f.Bills, f.Orders)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFirstBillWriteRollsBackEveryPublicAndEncryptedChange(t *testing.T) {
	s, path, key, _ := initializedFinance(t)
	beforeMain, _ := os.ReadFile(path)
	beforeBackup, _ := os.ReadFile(path + backupSuffix)
	var before register.Register
	s.Read(func(r *register.Register) { before = *deepCopy(r) })
	want := errors.New("stop")
	err := s.UpdateFinance(key, func(r *register.Register, f *register.FinanceData) error {
		party, _ := register.AddParty(r, "Verma Decor")
		challan, _ := register.AddSupplierChallan(r, party, "VD/91")
		purpose, _ := register.AddFinanceValue(f, register.FinancePurpose, "Event hire", "FAC-0001", financeNow)
		f.Bills = append(f.Bills, register.FinanceBill{
			ID: "BIL-0001", AmountPaise: 2000000, OccurredAt: financeNow, PartyID: party,
			PurposeID: purpose, SupplierChallanID: challan, RecordedAt: financeNow, RecordedByID: "FAC-0001",
			Products: []register.FinanceProductRef{{ProductID: "PRD-9999", ProductName: "Banquet chairs"}},
			Changes:  []register.FinanceChange{{Field: "remarks", Label: "Remarks"}},
		})
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	afterMain, _ := os.ReadFile(path)
	afterBackup, _ := os.ReadFile(path + backupSuffix)
	if !bytes.Equal(beforeMain, afterMain) || !bytes.Equal(beforeBackup, afterBackup) {
		t.Fatal("callback refusal changed main or backup")
	}
	s.Read(func(r *register.Register) {
		if !reflect.DeepEqual(&before, r) {
			t.Error("callback refusal changed in-memory register")
		}
	})
}

func TestDeepCopyFinanceCopiesBillNestedHistory(t *testing.T) {
	f := &register.FinanceData{Bills: []register.FinanceBill{{
		ID: "BIL-0001", Products: []register.FinanceProductRef{{ProductID: "PRD-0001", ProductName: "Chairs"}},
		Changes: []register.FinanceChange{{Field: "amount"}}, Voided: &register.FinanceVoid{Reason: "mistake"},
	}}}
	copy := deepCopyFinance(f)
	copy.Bills[0].Products[0].ProductName = "Tables"
	copy.Bills[0].Changes[0].Field = "remarks"
	copy.Bills[0].Voided.Reason = "changed"
	if reflect.DeepEqual(f, copy) {
		t.Fatal("bill nested state was shared")
	}
	if f.Bills[0].Products[0].ProductName != "Chairs" || f.Bills[0].Changes[0].Field != "amount" || f.Bills[0].Voided.Reason != "mistake" {
		t.Fatal("copy mutation reached original bill")
	}
}

func TestSupplierChallanRenameDeleteAndUseRules(t *testing.T) {
	s, _, key, _ := initializedFinance(t)
	var used, unused string
	if err := s.UpdateFinance(key, func(r *register.Register, f *register.FinanceData) error {
		party, _ := register.AddParty(r, "Sharma Tent House")
		used, _ = register.AddSupplierChallan(r, party, "STH/439O")
		unused, _ = register.AddSupplierChallan(r, party, "TYPO/1")
		purpose, _ := register.AddFinanceValue(f, register.FinancePurpose, "Rent", "FAC-0001", financeNow)
		f.Bills = append(f.Bills, register.FinanceBill{ID: "BIL-0001", AmountPaise: 2000000, OccurredAt: financeNow, PartyID: party, PurposeID: purpose, SupplierChallanID: used, RecordedAt: financeNow, RecordedByID: "FAC-0001"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameSupplierChallan(key, "FAC-0001", used, "STH/4390", financeNow.Add(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSupplierChallan(key, "FAC-0001", used, financeNow.Add(2)); !errors.Is(err, register.ErrSupplierChallanUsed) {
		t.Fatalf("used delete error = %v", err)
	}
	if err := s.DeleteSupplierChallan(key, "FAC-0001", unused, financeNow.Add(3)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReadBoth(key, func(r *register.Register, f *register.FinanceData) {
		row, ok := register.SupplierChallanByID(r, used)
		if !ok || row.Number != "STH/4390" {
			t.Errorf("renamed row = %+v, %v", row, ok)
		}
		if _, ok := register.SupplierChallanByID(r, unused); ok {
			t.Error("unused row remains")
		}
		if n := len(f.Audit); n < 3 || f.Audit[n-2].Kind != "supplier_challan_renamed" || f.Audit[n-1].Kind != "supplier_challan_deleted" {
			t.Errorf("audit tail = %+v", f.Audit)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
