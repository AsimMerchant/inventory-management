package store

import (
	"bytes"
	"os"
	"testing"
	"time"

	"storeregister/internal/register"
)

func TestReplaceUnconfirmedFinanceRecoveryIsNarrow(t *testing.T) {
	path := t.TempDir() + "/store-register.json"
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, register.IST)
	key, admin, old, err := s.InitializeFinance("Asha", "9886140023", "correct horse", now)
	if err != nil {
		t.Fatal(err)
	}

	replacement, err := s.ReplaceUnconfirmedFinanceRecovery(key, admin, now.Add(time.Minute))
	if err != nil || replacement == "" || replacement == old {
		t.Fatalf("replacement=%q err=%v", replacement, err)
	}
	if _, err := s.UnlockFinanceRecovery(old); err == nil {
		t.Fatal("abandoned recovery key still works")
	}
	if err := s.ConfirmFinanceRecovery(key, admin, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceUnconfirmedFinanceRecovery(key, admin, now.Add(3*time.Minute)); err != ErrLoginFailed {
		t.Fatalf("confirmed vault forced replacement err=%v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused forced replacement changed the file")
	}
}

func TestReplaceUnconfirmedFinanceRecoveryRefusesAnotherAdministrator(t *testing.T) {
	path := t.TempDir() + "/store-register.json"
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, register.IST)
	key, first, _, err := s.InitializeFinance("Asha", "9886140023", "correct horse", now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.AuthorizeFinanceAccount(key, first, "Meera", "9000011111", register.FinanceAdmin, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceUnconfirmedFinanceRecovery(key, second, now); err != ErrLoginFailed {
		t.Fatalf("second administrator forced replacement err=%v", err)
	}
}
