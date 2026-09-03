package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"storeregister/internal/register"
)

var financeNow = time.Date(2026, 9, 2, 12, 0, 0, 0, register.IST)

func initializedFinance(t *testing.T) (*Store, string, []byte, string) {
	t.Helper()
	s, _, path := openTemp(t)
	key, id, recovery, err := s.InitializeFinance("Asha Mehta", "98861 40023", "correct horse", financeNow)
	if err != nil {
		t.Fatal(err)
	}
	if id != "FAC-0001" {
		t.Fatalf("id=%q", id)
	}
	return s, path, key, recovery
}

func TestSchemaTwoLoadsAsCurrentWithoutWriting(t *testing.T) {
	r := register.WalkthroughT0()
	r.SchemaVersion = 2
	data := mustEncode(t, r)
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Read(func(got *register.Register) {
		if got.SchemaVersion != register.SchemaVersion || got.Finance != nil {
			t.Errorf("schema=%d finance=%v", got.SchemaVersion, got.Finance)
		}
	})
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("open wrote the schema-2 file")
	}
	if _, err := os.Stat(path + backupSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("open created backup")
	}
}

func TestFinanceVaultRoundTripContainsNoPlaintext(t *testing.T) {
	s, path, key, recovery := initializedFinance(t)
	if err := s.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		f.Audit = append(f.Audit, register.FinanceAuditEvent{ID: f.NextID("FAE"), At: financeNow, ByAccountID: "FAC-0001", ByName: "Asha Mehta", ByMobile: "98861 40023", Kind: "account_edited", EntityType: "account", EntityID: "FAC-0001", Summary: "Protected vault marker 7QX9"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{path, path + backupSuffix} {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"Asha", "98861", "Protected vault marker 7QX9"} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Errorf("%s contains %q", filename, forbidden)
			}
		}
	}
	reopened, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	unlocked, id, err := reopened.UnlockFinance("9886140023", "correct horse")
	if err != nil || id != "FAC-0001" {
		t.Fatalf("unlock id=%q err=%v", id, err)
	}
	if err := reopened.ReadFinance(unlocked, func(f *register.FinanceData) {
		if f.Accounts[0].DisplayName != "Asha Mehta" || f.Audit[len(f.Audit)-1].Summary != "Protected vault marker 7QX9" {
			t.Error("decrypted values differ")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.UnlockFinanceRecovery(recovery); err != nil {
		t.Fatal(err)
	}
}

func TestFinanceUsesFreshAuthenticatedEncryption(t *testing.T) {
	s, _, key, _ := initializedFinance(t)
	var first register.FinanceEnvelope
	s.Read(func(r *register.Register) { first = *r.Finance })
	if err := s.UpdateFinance(key, func(_ *register.Register, _ *register.FinanceData) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var second register.FinanceEnvelope
	s.Read(func(r *register.Register) { second = *r.Finance })
	if first.Nonce == second.Nonce || first.Ciphertext == second.Ciphertext {
		t.Fatal("save reused authenticated encryption")
	}
	if _, err := openSeal(key, second.Nonce, second.Ciphertext, vaultAAD+" changed"); err == nil {
		t.Fatal("changed vault AAD authenticated")
	}
	for name, breakEnvelope := range map[string]func(*register.FinanceEnvelope){
		"nonce":      func(e *register.FinanceEnvelope) { e.Nonce = strings.Repeat("A", len(e.Nonce)) },
		"ciphertext": func(e *register.FinanceEnvelope) { e.Ciphertext = e.Ciphertext[:len(e.Ciphertext)-2] + "AA" },
		"wrapped":    func(e *register.FinanceEnvelope) { e.KeySlots[0].WrappedKey = "AAAA" },
	} {
		t.Run(name, func(t *testing.T) {
			e := second
			e.KeySlots = append([]register.FinanceKeySlot{}, second.KeySlots...)
			breakEnvelope(&e)
			if name == "wrapped" {
				if _, err := unwrapSecret(e.KeySlots[0], "correct horse"); err == nil {
					t.Fatal("changed wrapper authenticated")
				}
				return
			}
			if _, err := decryptFinance(&e, key); !errors.Is(err, ErrLoginFailed) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestWrongPasswordAndRecoveryAreIndistinguishable(t *testing.T) {
	s, _, key, _ := initializedFinance(t)
	_, _, loginErr := s.UnlockFinance("98861 40023", "wrong password")
	_, code, err := s.AuthorizeFinanceAccount(key, "FAC-0001", "Rohan", "9900134562", register.FinanceUser, financeNow)
	if err != nil || code == "" { t.Fatal(err) }
	_, _, setupErr := s.ActivateFinance("9900134562", "WRONG-WRONG-WRONG", "long enough", financeNow)
	_, recoveryErr := s.UnlockFinanceRecovery("AAAA-BBBB")
	if loginErr.Error() != ErrLoginFailed.Error() || setupErr.Error() != ErrLoginFailed.Error() || recoveryErr.Error() != ErrLoginFailed.Error() {
		t.Fatalf("login=%q setup=%q recovery=%q", loginErr, setupErr, recoveryErr)
	}
}

func TestFinanceUpdateFailureRollsBackEverything(t *testing.T) {
	s, path, key, _ := initializedFinance(t)
	beforeMain, _ := os.ReadFile(path)
	var before register.Register
	s.Read(func(r *register.Register) { before = *deepCopy(r) })
	want := errors.New("stop")
	err := s.UpdateFinance(key, func(r *register.Register, f *register.FinanceData) error {
		r.Products = append(r.Products, register.Product{ID: "PRD-9999", Name: "leak"})
		f.Accounts[0].DisplayName = "leak"
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	afterMain, _ := os.ReadFile(path)
	if !bytes.Equal(beforeMain, afterMain) {
		t.Fatal("disk changed")
	}
	s.Read(func(r *register.Register) {
		if !reflect.DeepEqual(&before, r) {
			t.Error("memory changed")
		}
	})

	assertUnchanged := func(label string) {
		t.Helper()
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(beforeMain, after) {
			t.Errorf("%s changed disk", label)
		}
		s.Read(func(r *register.Register) {
			if !reflect.DeepEqual(&before, r) {
				t.Errorf("%s changed memory", label)
			}
		})
	}
	if err := s.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		f.Accounts = append(f.Accounts, f.Accounts[0])
		return nil
	}); err == nil {
		t.Fatal("invalid finance was saved")
	}
	assertUnchanged("validation failure")

	oldRandom := financeRandom
	financeRandom = strings.NewReader("")
	err = s.UpdateFinance(key, func(_ *register.Register, _ *register.FinanceData) error { return nil })
	financeRandom = oldRandom
	if err == nil {
		t.Fatal("random failure was ignored")
	}
	assertUnchanged("random failure")

	if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
		t.Fatal(err)
	}
	err = s.UpdateFinance(key, func(_ *register.Register, f *register.FinanceData) error {
		f.Accounts[0].DisplayName = "Cannot save"
		return nil
	})
	if chmodErr := os.Chmod(filepath.Dir(path), 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("forced save failure was ignored")
	}
	assertUnchanged("save failure")
}
