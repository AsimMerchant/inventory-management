package register

import "time"

type FinanceEnvelope struct {
	VaultVersion int              `json:"vaultVersion"`
	KeySlots     []FinanceKeySlot `json:"keySlots"`
	Recovery     FinanceKeySlot   `json:"recovery"`
	Nonce        string           `json:"nonce"`
	Ciphertext   string           `json:"ciphertext"`
}

type FinanceKeySlot struct {
	AccountID  string     `json:"accountId"`
	Kind       string     `json:"kind"`
	MobileHash string     `json:"mobileHash,omitempty"`
	Salt       string     `json:"salt,omitempty"`
	Iterations int        `json:"iterations,omitempty"`
	Nonce      string     `json:"nonce"`
	WrappedKey string     `json:"wrappedKey"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
}

type FinanceData struct {
	Accounts            []FinanceAccount    `json:"accounts"`
	Audit               []FinanceAuditEvent `json:"audit"`
	RecoveryConfirmedAt *time.Time          `json:"recoveryConfirmedAt,omitempty"`
}

type FinanceRole string

const (
	FinanceUser  FinanceRole = "user"
	FinanceAdmin FinanceRole = "admin"
)

type FinanceAccount struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"displayName"`
	Mobile      string      `json:"mobile"`
	Role        FinanceRole `json:"role"`
	Status      string      `json:"status"`
	CreatedAt   time.Time   `json:"createdAt"`
	CreatedByID string      `json:"createdById"`
}

type FinanceAuditEvent struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	ByAccountID string    `json:"byAccountId"`
	ByName      string    `json:"byName"`
	ByMobile    string    `json:"byMobile"`
	Kind        string    `json:"kind"`
	EntityType  string    `json:"entityType"`
	EntityID    string    `json:"entityId"`
	Summary     string    `json:"summary"`
	Before      string    `json:"before,omitempty"`
	After       string    `json:"after,omitempty"`
}
