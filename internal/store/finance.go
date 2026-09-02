package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"storeregister/internal/register"
)

const (
	vaultVersion  = 1
	keyIterations = 600000
	vaultAAD      = "store-register finance vault v1"
)

var (
	ErrLoginFailed           = errors.New("Those login details did not work.")
	financeRandom  io.Reader = rand.Reader
)

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(financeRandom, b); err != nil {
		return nil, err
	}
	return b, nil
}

func mobileHash(mobile string) string {
	sum := sha256.Sum256([]byte(register.MobileKey(mobile)))
	return hex.EncodeToString(sum[:])
}

func b64(b []byte) string            { return base64.RawStdEncoding.EncodeToString(b) }
func unb64(s string) ([]byte, error) { return base64.RawStdEncoding.DecodeString(s) }

func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(key, plaintext []byte, aad string) (string, string, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomBytes(gcm.NonceSize())
	if err != nil {
		return "", "", err
	}
	return b64(nonce), b64(gcm.Seal(nil, nonce, plaintext, []byte(aad))), nil
}

func openSeal(key []byte, nonceText, ciphertextText, aad string) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	nonce, err := unb64(nonceText)
	if err != nil {
		return nil, err
	}
	ciphertext, err := unb64(ciphertextText)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, []byte(aad))
}

func wrapAAD(accountID, kind string) string {
	return "store-register key wrap v1:" + accountID + ":" + kind
}

func passwordSlot(accountID, kind, mobile, secret string, vaultKey []byte, expires *time.Time) (register.FinanceKeySlot, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return register.FinanceKeySlot{}, err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, keyIterations, 32)
	if err != nil {
		return register.FinanceKeySlot{}, err
	}
	nonce, wrapped, err := seal(key, vaultKey, wrapAAD(accountID, kind))
	if err != nil {
		return register.FinanceKeySlot{}, err
	}
	return register.FinanceKeySlot{AccountID: accountID, Kind: kind, MobileHash: mobileHash(mobile), Salt: b64(salt), Iterations: keyIterations, Nonce: nonce, WrappedKey: wrapped, ExpiresAt: expires}, nil
}

func unwrapSecret(slot register.FinanceKeySlot, secret string) ([]byte, error) {
	salt, err := unb64(slot.Salt)
	if err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, slot.Iterations, 32)
	if err != nil {
		return nil, err
	}
	return openSeal(key, slot.Nonce, slot.WrappedKey, wrapAAD(slot.AccountID, slot.Kind))
}

func recoveryText(key []byte) string {
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
	parts := make([]string, 0, (len(raw)+3)/4)
	for len(raw) > 4 {
		parts = append(parts, raw[:4])
		raw = raw[4:]
	}
	if raw != "" {
		parts = append(parts, raw)
	}
	return strings.Join(parts, "-")
}

func parseRecovery(s string) ([]byte, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

func normalizeFinance(f *register.FinanceData) {
	if f.Accounts == nil {
		f.Accounts = []register.FinanceAccount{}
	}
	if f.Audit == nil {
		f.Audit = []register.FinanceAuditEvent{}
	}
	if f.Orders == nil {
		f.Orders = []register.FinanceOrder{}
	}
	if f.ReusableValues == nil {
		f.ReusableValues = []register.FinanceReusableValue{}
	}
	if f.Movements == nil {
		f.Movements = []register.MoneyMovement{}
	}
}

func decryptFinance(env *register.FinanceEnvelope, vaultKey []byte) (*register.FinanceData, error) {
	if env == nil || env.VaultVersion != vaultVersion {
		return nil, ErrLoginFailed
	}
	plain, err := openSeal(vaultKey, env.Nonce, env.Ciphertext, vaultAAD)
	if err != nil {
		return nil, ErrLoginFailed
	}
	var data register.FinanceData
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, ErrLoginFailed
	}
	normalizeFinance(&data)
	if err := register.ValidateFinance(&data); err != nil {
		return nil, ErrLoginFailed
	}
	return &data, nil
}

func encryptFinance(env *register.FinanceEnvelope, data *register.FinanceData, vaultKey []byte) error {
	plain, err := json.Marshal(data)
	if err != nil {
		return err
	}
	nonce, ciphertext, err := seal(vaultKey, plain, vaultAAD)
	if err != nil {
		return err
	}
	env.VaultVersion, env.Nonce, env.Ciphertext = vaultVersion, nonce, ciphertext
	return nil
}

func validPassword(password string) bool { return utf8.RuneCountInString(password) >= 8 }

func (s *Store) InitializeFinance(displayName, mobile, password string, now time.Time) ([]byte, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reg.Finance != nil {
		return nil, "", "", fmt.Errorf("authorized access is already set up")
	}
	displayName, mobile = register.CleanName(displayName), register.CleanName(mobile)
	if displayName == "" || register.MobileKey(mobile) == "" || !validPassword(password) {
		return nil, "", "", fmt.Errorf("name, mobile and an 8-character password are required")
	}
	vaultKey, err := randomBytes(32)
	if err != nil {
		return nil, "", "", err
	}
	recoveryKey, err := randomBytes(32)
	if err != nil {
		return nil, "", "", err
	}
	accountID := "FAC-0001"
	passwordWrap, err := passwordSlot(accountID, "password", mobile, password, vaultKey, nil)
	if err != nil {
		return nil, "", "", err
	}
	recoveryNonce, recoveryWrapped, err := seal(recoveryKey, vaultKey, wrapAAD("recovery", "recovery"))
	if err != nil {
		return nil, "", "", err
	}
	data := &register.FinanceData{Accounts: []register.FinanceAccount{{ID: accountID, DisplayName: displayName, Mobile: mobile, Role: register.FinanceAdmin, Status: "active", CreatedAt: now, CreatedByID: accountID}}, Audit: []register.FinanceAuditEvent{}}
	data.Audit = append(data.Audit, accountAudit(data, accountID, now, "account_created", accountID, "Authorized account created", "", "name="+displayName+", mobile="+mobile+", role=admin, status=active"))
	normalizeFinance(data)
	// The five modes everyone already uses, so the first payment does not begin
	// with the person inventing a list. Parties and purposes start empty
	// because there is nothing sensible to guess.
	for _, mode := range register.InitialPaymentModes {
		if _, err := register.AddFinanceValue(data, register.FinanceMode, mode, accountID, now); err != nil {
			return nil, "", "", err
		}
	}
	if err := register.ValidateFinance(data); err != nil {
		return nil, "", "", err
	}
	if problems := register.Validate(s.reg); len(problems) != 0 {
		return nil, "", "", fmt.Errorf("inventory validation failed")
	}
	env := &register.FinanceEnvelope{VaultVersion: vaultVersion, KeySlots: []register.FinanceKeySlot{passwordWrap}, Recovery: register.FinanceKeySlot{AccountID: "recovery", Kind: "recovery", Nonce: recoveryNonce, WrappedKey: recoveryWrapped}}
	if err := encryptFinance(env, data, vaultKey); err != nil {
		return nil, "", "", err
	}
	snapshot := deepCopy(s.reg)
	s.reg.Finance = env
	if err := s.save(); err != nil {
		s.reg = snapshot
		return nil, "", "", err
	}
	return append([]byte(nil), vaultKey...), accountID, recoveryText(recoveryKey), nil
}

func (s *Store) UnlockFinance(mobile, password string) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reg.Finance == nil {
		return nil, "", ErrLoginFailed
	}
	want := mobileHash(mobile)
	for _, slot := range s.reg.Finance.KeySlots {
		if slot.Kind != "password" || slot.MobileHash != want {
			continue
		}
		key, err := unwrapSecret(slot, password)
		if err != nil {
			continue
		}
		data, err := decryptFinance(s.reg.Finance, key)
		if err != nil {
			continue
		}
		for _, a := range data.Accounts {
			if a.ID == slot.AccountID && a.Status == "active" && register.MobileKey(a.Mobile) == register.MobileKey(mobile) {
				return append([]byte(nil), key...), a.ID, nil
			}
		}
	}
	return nil, "", ErrLoginFailed
}

func (s *Store) UnlockFinanceRecovery(recovery string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reg.Finance == nil {
		return nil, ErrLoginFailed
	}
	key, err := parseRecovery(recovery)
	if err != nil || len(key) != 32 {
		return nil, ErrLoginFailed
	}
	vaultKey, err := openSeal(key, s.reg.Finance.Recovery.Nonce, s.reg.Finance.Recovery.WrappedKey, wrapAAD("recovery", "recovery"))
	if err != nil {
		return nil, ErrLoginFailed
	}
	if _, err := decryptFinance(s.reg.Finance, vaultKey); err != nil {
		return nil, ErrLoginFailed
	}
	return append([]byte(nil), vaultKey...), nil
}

func (s *Store) ReadFinance(vaultKey []byte, fn func(*register.FinanceData)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := decryptFinance(s.reg.Finance, vaultKey)
	if err != nil {
		return err
	}
	copyData := deepCopyFinance(data)
	fn(copyData)
	return nil
}

func (s *Store) UpdateFinance(vaultKey []byte, fn func(*register.Register, *register.FinanceData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := decryptFinance(s.reg.Finance, vaultKey)
	if err != nil {
		return err
	}
	snapshot := deepCopy(s.reg)
	work := deepCopy(s.reg)
	envelope := deepCopy(s.reg).Finance
	workData := deepCopyFinance(data)
	if err := fn(work, workData); err != nil {
		return err
	}
	// Key slots are changed only by the private account transactions below.
	// General financial callbacks own decrypted data and inventory projections,
	// not the credential envelope.
	work.Finance = envelope
	if problems := register.Validate(work); len(problems) != 0 {
		return fmt.Errorf("inventory validation failed")
	}
	if err := register.ValidateFinance(workData); err != nil {
		return err
	}
	if err := encryptFinance(work.Finance, workData, vaultKey); err != nil {
		return err
	}
	s.reg = work
	if err := s.save(); err != nil {
		s.reg = snapshot
		return err
	}
	return nil
}

func deepCopyFinance(f *register.FinanceData) *register.FinanceData {
	c := *f
	c.Accounts = append([]register.FinanceAccount{}, f.Accounts...)
	c.Audit = append([]register.FinanceAuditEvent{}, f.Audit...)
	// Orders and values carry their own slices and a pointer. Sharing those
	// backing arrays would let a refused transaction leave its half-applied
	// edits behind in the live decrypted data.
	c.Orders = append([]register.FinanceOrder{}, f.Orders...)
	for i := range c.Orders {
		c.Orders[i].Lines = append([]register.FinanceOrderLine{}, f.Orders[i].Lines...)
		c.Orders[i].Changes = append([]register.FinanceChange{}, f.Orders[i].Changes...)
		if f.Orders[i].AgreedPaise != nil {
			v := *f.Orders[i].AgreedPaise
			c.Orders[i].AgreedPaise = &v
		}
	}
	c.ReusableValues = append([]register.FinanceReusableValue{}, f.ReusableValues...)
	for i := range c.ReusableValues {
		c.ReusableValues[i].Changes = append([]register.FinanceChange{}, f.ReusableValues[i].Changes...)
	}
	// A movement carries four nested things. Sharing any of them would let a
	// refused transaction leave half-applied edits in the live decrypted data.
	c.Movements = append([]register.MoneyMovement{}, f.Movements...)
	for i := range c.Movements {
		c.Movements[i].OrderLineIDs = append([]string{}, f.Movements[i].OrderLineIDs...)
		c.Movements[i].Products = append([]register.FinanceProductRef{}, f.Movements[i].Products...)
		c.Movements[i].Changes = append([]register.FinanceChange{}, f.Movements[i].Changes...)
		if f.Movements[i].Voided != nil {
			v := *f.Movements[i].Voided
			c.Movements[i].Voided = &v
		}
	}
	if f.RecoveryConfirmedAt != nil {
		t := *f.RecoveryConfirmedAt
		c.RecoveryConfirmedAt = &t
	}
	return &c
}

func accountAudit(data *register.FinanceData, actorID string, at time.Time, kind, entityID, summary, before, after string) register.FinanceAuditEvent {
	actor := register.FinanceAccount{ID: actorID}
	for _, a := range data.Accounts {
		if a.ID == actorID {
			actor = a
			break
		}
	}
	return register.FinanceAuditEvent{ID: data.NextID("FAE"), At: at, ByAccountID: actorID, ByName: actor.DisplayName, ByMobile: actor.Mobile, Kind: kind, EntityType: "account", EntityID: entityID, Summary: summary, Before: before, After: after}
}

func setupCode() (string, error) {
	b, err := randomBytes(13)
	if err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)[:20], nil
}

func accountByID(data *register.FinanceData, id string) (*register.FinanceAccount, bool) {
	for i := range data.Accounts {
		if data.Accounts[i].ID == id {
			return &data.Accounts[i], true
		}
	}
	return nil, false
}

func activeAdmin(data *register.FinanceData, id string) bool {
	a, ok := accountByID(data, id)
	return ok && a.Status == "active" && a.Role == register.FinanceAdmin
}

func activeAdminCount(data *register.FinanceData) int {
	n := 0
	for _, a := range data.Accounts {
		if a.Status == "active" && a.Role == register.FinanceAdmin {
			n++
		}
	}
	return n
}

func (s *Store) FinanceExists() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reg.Finance != nil
}

func (s *Store) ConfirmFinanceRecovery(vaultKey []byte, accountID string, now time.Time) error {
	return s.UpdateFinance(vaultKey, func(_ *register.Register, data *register.FinanceData) error {
		if !activeAdmin(data, accountID) {
			return ErrLoginFailed
		}
		t := now
		data.RecoveryConfirmedAt = &t
		return nil
	})
}

func (s *Store) AuthorizeFinanceAccount(vaultKey []byte, actorID, displayName, mobile string, role register.FinanceRole, now time.Time) (string, string, error) {
	code, err := setupCode()
	if err != nil {
		return "", "", err
	}
	displayName, mobile = register.CleanName(displayName), register.CleanName(mobile)
	var id string
	err = s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		if !activeAdmin(data, actorID) {
			return fmt.Errorf("administrator access is required")
		}
		if displayName == "" || register.MobileKey(mobile) == "" || (role != register.FinanceUser && role != register.FinanceAdmin) {
			return fmt.Errorf("name, mobile and role are required")
		}
		for _, a := range data.Accounts {
			if register.MobileKey(a.Mobile) == register.MobileKey(mobile) {
				return fmt.Errorf("that mobile number is already authorized")
			}
		}
		id = data.NextID("FAC")
		a := register.FinanceAccount{ID: id, DisplayName: displayName, Mobile: mobile, Role: role, Status: "pending", CreatedAt: now, CreatedByID: actorID}
		data.Accounts = append(data.Accounts, a)
		expires := now.Add(24 * time.Hour)
		slot, err := passwordSlot(id, "setup", mobile, code, vaultKey, &expires)
		if err != nil {
			return err
		}
		env.KeySlots = append(env.KeySlots, slot)
		data.Audit = append(data.Audit, accountAudit(data, actorID, now, "account_created", id, "Authorized account created", "", "name="+displayName+", mobile="+mobile+", role="+string(role)+", status=pending"))
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return id, code, nil
}

func (s *Store) ActivateFinance(mobile, code, password string, now time.Time) ([]byte, string, error) {
	if !validPassword(password) {
		return nil, "", fmt.Errorf("password must be at least 8 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	env := s.reg.Finance
	if env == nil {
		return nil, "", ErrLoginFailed
	}
	want := mobileHash(mobile)
	for _, setup := range env.KeySlots {
		if setup.Kind != "setup" || setup.MobileHash != want || setup.ExpiresAt == nil || !now.Before(*setup.ExpiresAt) {
			continue
		}
		vaultKey, err := unwrapSecret(setup, code)
		if err != nil {
			continue
		}
		data, err := decryptFinance(env, vaultKey)
		if err != nil {
			continue
		}
		a, ok := accountByID(data, setup.AccountID)
		if !ok || a.Status != "pending" || register.MobileKey(a.Mobile) != register.MobileKey(mobile) {
			continue
		}
		newSlot, err := passwordSlot(a.ID, "password", a.Mobile, password, vaultKey, nil)
		if err != nil {
			return nil, "", err
		}
		work := deepCopy(s.reg)
		workData := deepCopyFinance(data)
		wa, _ := accountByID(workData, a.ID)
		wa.Status = "active"
		work.Finance.KeySlots = removeAccountSlots(work.Finance.KeySlots, a.ID)
		work.Finance.KeySlots = append(work.Finance.KeySlots, newSlot)
		workData.Audit = append(workData.Audit, accountAudit(workData, a.ID, now, "account_activated", a.ID, "Authorized account activated", "status=pending", "status=active"))
		if err := s.publishFinance(work, workData, vaultKey); err != nil {
			return nil, "", err
		}
		return append([]byte(nil), vaultKey...), a.ID, nil
	}
	return nil, "", ErrLoginFailed
}

func removeAccountSlots(slots []register.FinanceKeySlot, id string) []register.FinanceKeySlot {
	out := make([]register.FinanceKeySlot, 0, len(slots))
	for _, slot := range slots {
		if slot.AccountID != id {
			out = append(out, slot)
		}
	}
	return out
}

func (s *Store) DisableFinanceAccount(vaultKey []byte, actorID, targetID string, now time.Time) error {
	return s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		if !activeAdmin(data, actorID) {
			return fmt.Errorf("administrator access is required")
		}
		a, ok := accountByID(data, targetID)
		if !ok {
			return fmt.Errorf("authorized account not found")
		}
		if a.Status == "active" && a.Role == register.FinanceAdmin && activeAdminCount(data) == 1 {
			return errors.New("Keep at least one financial administrator active.")
		}
		before := "status=" + a.Status
		a.Status = "disabled"
		env.KeySlots = removeAccountSlots(env.KeySlots, targetID)
		data.Audit = append(data.Audit, accountAudit(data, actorID, now, "account_disabled", targetID, "Authorized account disabled", before, "status=disabled"))
		return nil
	})
}

func (s *Store) ResetFinanceAccount(vaultKey []byte, actorID, targetID string, now time.Time) (string, error) {
	code, err := setupCode()
	if err != nil {
		return "", err
	}
	err = s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		if !activeAdmin(data, actorID) {
			return fmt.Errorf("administrator access is required")
		}
		a, ok := accountByID(data, targetID)
		if !ok || a.Status == "disabled" {
			return fmt.Errorf("authorized account not found")
		}
		expires := now.Add(24 * time.Hour)
		slot, err := passwordSlot(a.ID, "setup", a.Mobile, code, vaultKey, &expires)
		if err != nil {
			return err
		}
		env.KeySlots = append(removeAccountSlots(env.KeySlots, targetID), slot)
		before := "status=" + a.Status
		a.Status = "pending"
		data.Audit = append(data.Audit, accountAudit(data, actorID, now, "account_reset", targetID, "Authorized account reset", before, "status=pending"))
		return nil
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

func (s *Store) EditFinanceAccount(vaultKey []byte, actorID, targetID, name, mobile string, role register.FinanceRole, now time.Time) error {
	name, mobile = register.CleanName(name), register.CleanName(mobile)
	return s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		if !activeAdmin(data, actorID) {
			return fmt.Errorf("administrator access is required")
		}
		a, ok := accountByID(data, targetID)
		if !ok {
			return fmt.Errorf("authorized account not found")
		}
		if name == "" || register.MobileKey(mobile) == "" || (role != register.FinanceUser && role != register.FinanceAdmin) {
			return fmt.Errorf("name, mobile and role are required")
		}
		for _, other := range data.Accounts {
			if other.ID != targetID && register.MobileKey(other.Mobile) == register.MobileKey(mobile) {
				return fmt.Errorf("that mobile number is already authorized")
			}
		}
		if a.Status == "active" && a.Role == register.FinanceAdmin && role != register.FinanceAdmin && activeAdminCount(data) == 1 {
			return errors.New("Keep at least one financial administrator active.")
		}
		before := "name=" + a.DisplayName + ", mobile=" + a.Mobile + ", role=" + string(a.Role) + ", status=" + a.Status
		a.DisplayName, a.Mobile, a.Role = name, mobile, role
		for i := range env.KeySlots {
			if env.KeySlots[i].AccountID == targetID {
				env.KeySlots[i].MobileHash = mobileHash(mobile)
			}
		}
		after := "name=" + a.DisplayName + ", mobile=" + a.Mobile + ", role=" + string(a.Role) + ", status=" + a.Status
		data.Audit = append(data.Audit, accountAudit(data, actorID, now, "account_edited", targetID, "Authorized account corrected", before, after))
		return nil
	})
}

func (s *Store) ChangeFinancePassword(vaultKey []byte, accountID, current, password string, now time.Time) error {
	if !validPassword(password) {
		return fmt.Errorf("password must be at least 8 characters")
	}
	return s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		a, ok := accountByID(data, accountID)
		if !ok || a.Status != "active" {
			return ErrLoginFailed
		}
		var verified bool
		for _, slot := range env.KeySlots {
			if slot.AccountID == accountID && slot.Kind == "password" {
				key, err := unwrapSecret(slot, current)
				verified = err == nil && bytesEqual(key, vaultKey)
			}
		}
		if !verified {
			return ErrLoginFailed
		}
		slot, err := passwordSlot(accountID, "password", a.Mobile, password, vaultKey, nil)
		if err != nil {
			return err
		}
		env.KeySlots = append(removeAccountSlots(env.KeySlots, accountID), slot)
		data.Audit = append(data.Audit, accountAudit(data, accountID, now, "password_changed", accountID, "Password changed", "", ""))
		return nil
	})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func (s *Store) RecoverFinanceAdministrator(recovery, mobile, password string, now time.Time) ([]byte, string, error) {
	if !validPassword(password) {
		return nil, "", fmt.Errorf("password must be at least 8 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reg.Finance == nil {
		return nil, "", ErrLoginFailed
	}
	recoveryKey, err := parseRecovery(recovery)
	if err != nil || len(recoveryKey) != 32 {
		return nil, "", ErrLoginFailed
	}
	vaultKey, err := openSeal(recoveryKey, s.reg.Finance.Recovery.Nonce, s.reg.Finance.Recovery.WrappedKey, wrapAAD("recovery", "recovery"))
	if err != nil {
		return nil, "", ErrLoginFailed
	}
	data, err := decryptFinance(s.reg.Finance, vaultKey)
	if err != nil {
		return nil, "", ErrLoginFailed
	}
	var id string
	for _, candidate := range data.Accounts {
		if candidate.Status == "active" && candidate.Role == register.FinanceAdmin && register.MobileKey(candidate.Mobile) == register.MobileKey(mobile) {
			id = candidate.ID
			break
		}
	}
	if id == "" {
		return nil, "", ErrLoginFailed
	}
	a, _ := accountByID(data, id)
	slot, err := passwordSlot(id, "password", a.Mobile, password, vaultKey, nil)
	if err != nil {
		return nil, "", err
	}
	work := deepCopy(s.reg)
	workData := deepCopyFinance(data)
	work.Finance.KeySlots = append(removeAccountSlots(work.Finance.KeySlots, id), slot)
	workData.Audit = append(workData.Audit, accountAudit(workData, id, now, "account_recovered", id, "Authorized account recovered", "", ""))
	if err := s.publishFinance(work, workData, vaultKey); err != nil {
		return nil, "", err
	}
	return vaultKey, id, nil
}

func (s *Store) ReplaceFinanceRecovery(vaultKey []byte, accountID, currentPassword string, now time.Time) (string, error) {
	recoveryKey, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	err = s.financeTransaction(vaultKey, func(env *register.FinanceEnvelope, data *register.FinanceData) error {
		a, ok := accountByID(data, accountID)
		if !ok || !activeAdmin(data, accountID) {
			return ErrLoginFailed
		}
		var verified bool
		for _, slot := range env.KeySlots {
			if slot.AccountID == accountID && slot.Kind == "password" {
				key, err := unwrapSecret(slot, currentPassword)
				verified = err == nil && bytesEqual(key, vaultKey)
			}
		}
		if !verified {
			return ErrLoginFailed
		}
		nonce, wrapped, err := seal(recoveryKey, vaultKey, wrapAAD("recovery", "recovery"))
		if err != nil {
			return err
		}
		env.Recovery = register.FinanceKeySlot{AccountID: "recovery", Kind: "recovery", Nonce: nonce, WrappedKey: wrapped}
		data.RecoveryConfirmedAt = nil
		e := accountAudit(data, accountID, now, "recovery_key_replaced", "recovery", "Recovery key replaced", "", "")
		e.EntityType = "recovery"
		data.Audit = append(data.Audit, e)
		_ = a
		return nil
	})
	if err != nil {
		return "", err
	}
	return recoveryText(recoveryKey), nil
}

func (s *Store) financeTransaction(vaultKey []byte, mutate func(*register.FinanceEnvelope, *register.FinanceData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := decryptFinance(s.reg.Finance, vaultKey)
	if err != nil {
		return err
	}
	work := deepCopy(s.reg)
	workData := deepCopyFinance(data)
	if err := mutate(work.Finance, workData); err != nil {
		return err
	}
	return s.publishFinance(work, workData, vaultKey)
}

func (s *Store) publishFinance(work *register.Register, data *register.FinanceData, vaultKey []byte) error {
	if problems := register.Validate(work); len(problems) != 0 {
		return fmt.Errorf("inventory validation failed")
	}
	if err := register.ValidateFinance(data); err != nil {
		return err
	}
	if err := encryptFinance(work.Finance, data, vaultKey); err != nil {
		return err
	}
	snapshot := s.reg
	s.reg = work
	if err := s.save(); err != nil {
		s.reg = snapshot
		return err
	}
	return nil
}
