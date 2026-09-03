package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

func TestImportVaultPartiesKeepsEveryReferenceAndNonPartyValue(t *testing.T) {
	at := time.Date(2026, 9, 3, 12, 0, 0, 0, register.IST)
	r := &register.Register{Inwards: []register.Inward{{
		ID: "INW-0001", Supplier: "Sharma Events", RecordedAt: at, RecordedBy: "Suresh",
	}}}
	register.LinkInwardParties(r)
	inwardParty := r.Inwards[0].PartyID
	f := &register.FinanceData{
		Accounts: []register.FinanceAccount{{ID: "FAC-0001", DisplayName: "Asha Mehta"}},
		ReusableValues: []register.FinanceReusableValue{
			{ID: "PTY-0001", Kind: register.FinanceParty, Value: "Sharma Events", CreatedAt: at, CreatedByID: "FAC-0001"},
			{ID: "PTY-0002", Kind: register.FinanceParty, Value: "Bala Transport", CreatedAt: at, CreatedByID: "FAC-0001"},
			{ID: "PUR-0001", Kind: register.FinancePurpose, Value: "Freight", CreatedAt: at, CreatedByID: "FAC-0001"},
		},
		Orders:          []register.FinanceOrder{{ID: "ORD-0001", PartyID: "PTY-0001"}},
		Movements:       []register.MoneyMovement{{ID: "MOV-0001", PartyID: "PTY-0002", PurposeID: "PUR-0001"}},
		SupplierReturns: []register.SupplierReturn{{ID: "SRN-0001", PartyID: "PTY-0001"}},
		Sales:           []register.StockSale{{ID: "SAL-0001", BuyerPartyID: "PTY-0002"}},
	}

	importVaultParties(r, f)

	if len(r.Parties) != 3 {
		t.Fatalf("migration kept %d party rows, want the inward row and two vault rows", len(r.Parties))
	}
	if got := register.ResolvedPartyID(r, "PTY-0001"); got != inwardParty {
		t.Fatalf("matching vault party resolves to %s, want inward party %s", got, inwardParty)
	}
	if got := register.PartyText(r, "PTY-0002"); got != "Bala Transport" {
		t.Fatalf("unmatched vault party resolves to %q", got)
	}
	if len(f.ReusableValues) != 1 || f.ReusableValues[0].ID != "PUR-0001" {
		t.Fatalf("non-party reusable values changed: %+v", f.ReusableValues)
	}
	if f.Orders[0].PartyID != "PTY-0001" || f.Movements[0].PartyID != "PTY-0002" ||
		f.SupplierReturns[0].PartyID != "PTY-0001" || f.Sales[0].BuyerPartyID != "PTY-0002" {
		t.Fatal("migration rewrote a financial record reference")
	}
	if err := register.ValidatePartyReferences(r, f); err != nil {
		t.Fatalf("migrated references are invalid: %v", err)
	}
}

func TestFirstFinanceWriteMigratesEncryptedSchemaFourPartiesAtomically(t *testing.T) {
	s, path, key, _ := initializedFinance(t)
	at := financeNow

	// Recreate the exact old shape: the party and every reference to it are
	// encrypted inside a schema-4 file.
	s.mu.Lock()
	old, err := decryptFinance(s.reg.Finance, key)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	partyID, err := register.AddFinanceValue(old, register.FinanceParty, "Sharma Events", "FAC-0001", at)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	old.Orders = append(old.Orders, register.FinanceOrder{
		ID: "ORD-0001", PartyID: partyID, OrderedAt: at, Status: "open", CreatedAt: at, CreatedByID: "FAC-0001",
		Lines: []register.FinanceOrderLine{{ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Tents", ExpectedQuantity: 50, Basis: register.Rent}},
	})
	if err := encryptFinance(s.reg.Finance, old, key); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.reg.SchemaVersion = 4
	if err := s.save(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("Sharma Events")) {
		t.Fatal("schema-4 party was not encrypted")
	}
	s, _, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err = s.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateFinance(key, func(_ *register.Register, _ *register.FinanceData) error { return nil }); err != nil {
		t.Fatalf("first schema-5 finance write: %v", err)
	}
	renameAt := at.Add(37 * time.Minute)
	if err := s.RenameParty(key, "FAC-0001", partyID, "Sharma Event Hire", renameAt); err != nil {
		t.Fatalf("rename migrated party: %v", err)
	}
	var migratedParty string
	s.Read(func(r *register.Register) {
		if r.SchemaVersion != register.SchemaVersion {
			t.Errorf("schema=%d", r.SchemaVersion)
		}
		migratedParty = register.PartyText(r, partyID)
	})
	if migratedParty != "Sharma Event Hire" {
		t.Fatalf("migrated party resolves to %q", migratedParty)
	}
	if err := s.ReadFinance(key, func(f *register.FinanceData) {
		if len(register.LiveFinanceValues(f, register.FinanceParty)) != 0 {
			t.Error("party remained duplicated inside the vault")
		}
		if len(f.Orders) != 1 || f.Orders[0].PartyID != partyID {
			t.Fatalf("order reference changed: %+v", f.Orders)
		}
		foundAudit := false
		for _, event := range f.Audit {
			if event.Kind == "party_renamed" && event.EntityID == partyID {
				foundAudit = event.ByAccountID == "FAC-0001" && event.ByName == "Asha Mehta" &&
					event.ByMobile == "98861 40023" && event.At.Equal(renameAt)
			}
		}
		if !foundAudit {
			t.Error("encrypted finance audit lost the rename actor or time")
		}
	}); err != nil {
		t.Fatal(err)
	}

	// The desk needs party IDs and names without unlocking finance. Nothing
	// about the financial account or its audit trail may leave the vault.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{partyID, "Sharma Events", "Sharma Event Hire"} {
		if !bytes.Contains(raw, []byte(visible)) {
			t.Errorf("public party data does not contain %q", visible)
		}
	}
	for _, secret := range []string{"Asha Mehta", "98861 40023", renameAt.Format(time.RFC3339), `"createdAt"`, `"createdBy"`, `"changes"`} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Errorf("public register contains protected party metadata %q", secret)
		}
	}
	var disk struct {
		Parties []map[string]json.RawMessage `json:"parties"`
	}
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if len(disk.Parties) != 1 {
		t.Fatalf("saved %d public parties, want 1", len(disk.Parties))
	}
	allowed := map[string]bool{"id": true, "name": true, "previousNames": true, "mergedIntoId": true}
	for field := range disk.Parties[0] {
		if !allowed[field] {
			t.Errorf("public party persisted protected field %q", field)
		}
	}

	// Both halves survive a real close/reopen and decrypt together.
	reopened, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	key2, _, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ReadBoth(key2, func(r *register.Register, f *register.FinanceData) {
		if register.PartyText(r, f.Orders[0].PartyID) != "Sharma Event Hire" {
			t.Error("restarted order lost its party")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFirstFinanceWritePartyMigrationRollsBackOnFailure(t *testing.T) {
	type fixture struct {
		s             *Store
		path, partyID string
		key           []byte
		main, backup  []byte
		memory        []byte
	}
	setup := func(t *testing.T) fixture {
		t.Helper()
		s, path, key, _ := initializedFinance(t)
		s.mu.Lock()
		old, err := decryptFinance(s.reg.Finance, key)
		if err != nil {
			s.mu.Unlock()
			t.Fatal(err)
		}
		partyID, err := register.AddFinanceValue(old, register.FinanceParty, "Sharma Events", "FAC-0001", financeNow)
		if err != nil {
			s.mu.Unlock()
			t.Fatal(err)
		}
		old.Orders = append(old.Orders, register.FinanceOrder{
			ID: "ORD-0001", PartyID: partyID, OrderedAt: financeNow, Status: "open",
			CreatedAt: financeNow, CreatedByID: "FAC-0001",
			Lines: []register.FinanceOrderLine{{
				ID: "OLN-0001", ProductID: "PRD-0001", ProductNameSnapshot: "Tents",
				ExpectedQuantity: 50, Basis: register.Rent,
			}},
		})
		if err := encryptFinance(s.reg.Finance, old, key); err != nil {
			s.mu.Unlock()
			t.Fatal(err)
		}
		s.reg.SchemaVersion = 4
		if err := s.save(); err != nil {
			s.mu.Unlock()
			t.Fatal(err)
		}
		memory, err := json.Marshal(s.reg)
		s.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		main, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		backup, err := os.ReadFile(path + backupSuffix)
		if err != nil {
			t.Fatal(err)
		}
		return fixture{s: s, path: path, partyID: partyID, key: key, main: main, backup: backup, memory: memory}
	}
	assertUnchanged := func(t *testing.T, x fixture) {
		t.Helper()
		main, err := os.ReadFile(x.path)
		if err != nil || !bytes.Equal(main, x.main) {
			t.Errorf("main changed: %v", err)
		}
		backup, err := os.ReadFile(x.path + backupSuffix)
		if err != nil || !bytes.Equal(backup, x.backup) {
			t.Errorf("backup changed: %v", err)
		}
		x.s.Read(func(r *register.Register) {
			memory, err := json.Marshal(r)
			if err != nil || !bytes.Equal(memory, x.memory) {
				t.Errorf("memory changed: %v", err)
			}
			if len(r.Parties) != 0 || r.SchemaVersion != 4 {
				t.Errorf("public migration leaked after refusal: schema=%d parties=%+v", r.SchemaVersion, r.Parties)
			}
		})
		if err := x.s.ReadFinance(x.key, func(f *register.FinanceData) {
			party, ok := register.FinanceValueByID(f, x.partyID)
			if !ok || party.Kind != register.FinanceParty || party.Value != "Sharma Events" {
				t.Errorf("encrypted party changed: %+v", party)
			}
			if len(f.Orders) != 1 || f.Orders[0].PartyID != x.partyID {
				t.Errorf("encrypted reference changed: %+v", f.Orders)
			}
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("callback refusal", func(t *testing.T) {
		x := setup(t)
		want := errors.New("refuse callback")
		if err := x.s.UpdateFinance(x.key, func(*register.Register, *register.FinanceData) error { return want }); !errors.Is(err, want) {
			t.Fatalf("error=%v", err)
		}
		assertUnchanged(t, x)
	})
	t.Run("cross-boundary validation", func(t *testing.T) {
		x := setup(t)
		if err := x.s.UpdateFinance(x.key, func(_ *register.Register, f *register.FinanceData) error {
			f.Orders[0].PartyID = "PTY-9999"
			return nil
		}); err == nil {
			t.Fatal("invalid party reference was saved")
		}
		assertUnchanged(t, x)
	})
	t.Run("encryption randomness", func(t *testing.T) {
		x := setup(t)
		oldRandom := financeRandom
		financeRandom = strings.NewReader("")
		defer func() { financeRandom = oldRandom }()
		if err := x.s.UpdateFinance(x.key, func(*register.Register, *register.FinanceData) error { return nil }); err == nil {
			t.Fatal("encryption failure was ignored")
		}
		assertUnchanged(t, x)
	})
	t.Run("atomic save", func(t *testing.T) {
		x := setup(t)
		dir := filepath.Dir(x.path)
		if err := os.Chmod(dir, 0500); err != nil {
			t.Fatal(err)
		}
		err := x.s.UpdateFinance(x.key, func(*register.Register, *register.FinanceData) error { return nil })
		if chmodErr := os.Chmod(dir, 0700); chmodErr != nil {
			t.Fatal(chmodErr)
		}
		if err == nil {
			t.Fatal("forced save failure was ignored")
		}
		assertUnchanged(t, x)
	})
}

func TestImportVaultPartyRenameAndMergeHistoryStillResolves(t *testing.T) {
	at := time.Date(2026, 9, 3, 12, 0, 0, 0, register.IST)
	r := &register.Register{}
	f := &register.FinanceData{
		Accounts: []register.FinanceAccount{{ID: "FAC-0001", DisplayName: "Asha Mehta"}},
		ReusableValues: []register.FinanceReusableValue{
			{ID: "PTY-0001", Kind: register.FinanceParty, Value: "Sharm Events", CreatedAt: at, CreatedByID: "FAC-0001", MergedIntoID: "PTY-0002"},
			{ID: "PTY-0002", Kind: register.FinanceParty, Value: "Sharma Tent House", CreatedAt: at, CreatedByID: "FAC-0001", Changes: []register.FinanceChange{{At: at, ByName: "Asha Mehta", Field: "value", Label: "Party", From: "Sharma Events", To: "Sharma Tent House"}}},
		},
	}

	importVaultParties(r, f)
	if got := register.PartyText(r, "PTY-0001"); got != "Sharma Tent House" {
		t.Fatalf("merged imported party resolves to %q", got)
	}
	aliases := register.PartyAliases(r, "PTY-0001")
	for _, want := range []string{"sharm events", "sharma events", "sharma tent house"} {
		if !aliases[want] {
			t.Errorf("imported history lost alias %q: %v", want, aliases)
		}
	}
}
