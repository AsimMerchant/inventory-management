// Package register holds the store's data model and its arithmetic.
// It does no I/O, opens no files and reads no clock: every function that
// depends on the time takes it as a parameter.
package register

import (
	"strings"
	"time"
)

// SchemaVersion is the only file version this program reads or writes.
const SchemaVersion = 5

// Register is everything the store desk remembers.
type Register struct {
	SchemaVersion  int        `json:"schemaVersion"`
	OnDutyStaffID  string     `json:"onDutyStaffId"` // "" when no shift started
	ShiftStartedAt *time.Time `json:"shiftStartedAt,omitempty"`
	Products       []Product  `json:"products"`
	Staff          []Staff    `json:"staff"`
	Inwards        []Inward   `json:"inwards"`
	Issues         []Issue    `json:"issues"`
	Returns        []Return   `json:"returns"`
	// Disposals is the neutral public record that some stock physically left
	// the store. It deliberately says nothing about whether that was a sale or
	// a return to a supplier, to whom, or for how much: that is in the vault.
	// It exists so ordinary stock arithmetic is right after a restart with
	// nobody logged in.
	Disposals []InventoryDisposal `json:"disposals"`
	// AcquisitionKinds is the shared vocabulary for goods that arrived neither
	// on rent nor by purchase: donated, sponsored, borrowed. It sits in the
	// open beside Disposals and for the same reason: the delivery desk is
	// never logged in, so a list it has to read cannot live in the vault. A
	// word like "donated" is not financial data.
	AcquisitionKinds []AcquisitionKind `json:"acquisitionKinds,omitempty"`
	// Parties is the shared list of suppliers and other parties, in the open
	// for the same reason: the desk picks a supplier off it and is never
	// logged in. It used to live inside the vault, where the desk could not
	// read it, so the two halves of the program kept two spellings of one
	// supplier. A name carries no amount, no purpose and no payment mode:
	// every figure stays in the vault.
	Parties []Party          `json:"parties,omitempty"`
	Finance *FinanceEnvelope `json:"finance,omitempty"`
}

// InventoryDisposal is one lot of stock that left the store for good.
type InventoryDisposal struct {
	ID         string               `json:"id"` // DSP-0001
	ProductID  string               `json:"productId"`
	Quantity   int                  `json:"quantity"`
	Sources    []DisposalAllocation `json:"sources"`
	RecordedAt time.Time            `json:"recordedAt"`
	// InactiveAt says only that the subtraction no longer applies. It cannot
	// reveal whether that happened through a protected void or a public
	// product deletion.
	InactiveAt *time.Time `json:"inactiveAt,omitempty"`
}

// DisposalAllocation attributes part of a disposal to the inward it came from.
type DisposalAllocation struct {
	InwardID string `json:"inwardId"`
	Quantity int    `json:"quantity"`
}

// Product is a thing the store stocks. Stock is pooled per product.
type Product struct {
	ID        string    `json:"id"` // "PRD-0001"
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"` // on-duty staff name when it was added
	Changes   []Change  `json:"changes,omitempty"`
	Deleted   *Deletion `json:"deleted,omitempty"`
}

// Staff is a person who can be on duty at the desk.
type Staff struct {
	ID        string    `json:"id"` // "STF-0001"
	Name      string    `json:"name"`
	Mobile    string    `json:"mobile"`
	CreatedAt time.Time `json:"createdAt"`
	// CreatedBy is the on-duty staff name when this person was added. It is
	// empty for the first person on a fresh register, because nobody was on
	// duty yet. That is a real value, not a missing one.
	CreatedBy string `json:"createdBy"`
}

// Change is one correction to one field of one record. Append-only.
type Change struct {
	At    time.Time `json:"at"`
	By    string    `json:"by"`    // on-duty staff name at the time of the correction
	Field string    `json:"field"` // "quantity" | "supplier" | "takerName" | ...
	Label string    `json:"label"` // the on-screen label, e.g. "How many"
	From  string    `json:"from"`  // rendered as the user saw it: "500"
	To    string    `json:"to"`    // "50"
}

// Deletion is a tombstone. The record stays in the file and counts towards nothing.
type Deletion struct {
	At     time.Time `json:"at"`
	By     string    `json:"by"`
	Reason string    `json:"reason"` // required, plain words
}

// Basis says how stock arrived. Rent and Purchase are the two the program
// knows about by name; Other means the desk typed its own word, and Inward.KindID
// says which one.
type Basis string

const (
	Rent     Basis = "rent"
	Purchase Basis = "purchase"
	Other    Basis = "other"
)

// Inward is one delivery into the store. It is not a lot: issues and returns
// never point at an inward record.
type Inward struct {
	ID         string `json:"id"`         // "INW-0001"
	ProductID  string `json:"productId"`  //
	Quantity   int    `json:"quantity"`   // >= 1
	ReceivedOn string `json:"receivedOn"` // "2026-09-03", date only, editable
	Basis      Basis  `json:"basis"`
	KindID     string `json:"kindId,omitempty"` // AKD-0001, only when Basis is Other
	// PartyID is the shared list entry these goods came from. Supplier is the
	// name as it stood when the delivery was saved: history, and what makes
	// the file readable by hand. A rename changes what the screens show
	// through PartyID and never rewrites Supplier.
	PartyID    string    `json:"partyId,omitempty"` // PRT-0001 or PTY-0001
	Supplier   string    `json:"supplier"`          // "" allowed
	ChallanNo  string    `json:"challanNo"`         // "" allowed
	ReceivedBy string    `json:"receivedBy"`        // "" allowed; defaults to on-duty name
	RecordedAt time.Time `json:"recordedAt"`
	RecordedBy string    `json:"recordedBy"`
	Changes    []Change  `json:"changes,omitempty"`
	Deleted    *Deletion `json:"deleted,omitempty"`
}

// Issue is one handover of stock to a person.
type Issue struct {
	ID                   string           `json:"id"` // "ISS-0001"
	ProductID            string           `json:"productId"`
	Quantity             int              `json:"quantity"` // >= 1
	ChallanNo            string           `json:"challanNo,omitempty"`
	TakerName            string           `json:"takerName"`
	TakerDepartment      string           `json:"takerDepartment"`
	TakerMobile          string           `json:"takerMobile"`
	AdditionalTakers     []IssueRecipient `json:"additionalTakers,omitempty"`
	PersonInchargeName   string           `json:"personInchargeName"`
	PersonInchargeMobile string           `json:"personInchargeMobile"`
	IssuedAt             time.Time        `json:"issuedAt"`   // auto-filled, editable
	RecordedAt           time.Time        `json:"recordedAt"` // never editable
	Changes              []Change         `json:"changes,omitempty"`
	Deleted              *Deletion        `json:"deleted,omitempty"`
}

// IssueRecipient is one named member of the set collecting an issue. The
// quantity belongs to the issue as a whole, never to an individual recipient.
type IssueRecipient struct {
	Name       string `json:"name"`
	Department string `json:"department"`
	Mobile     string `json:"mobile"`
}

// RecipientsOf returns the legacy first recipient followed by any additional
// recipients, in the order entered. The returned slice is independent of the
// issue's stored slice.
func RecipientsOf(is Issue) []IssueRecipient {
	out := make([]IssueRecipient, 0, 1+len(is.AdditionalTakers))
	out = append(out, IssueRecipient{Name: is.TakerName, Department: is.TakerDepartment, Mobile: is.TakerMobile})
	out = append(out, is.AdditionalTakers...)
	return out
}

// RecipientLabel names every recipient in ordinary prose, without an Oxford
// comma: "Ravi, Amit and Suresh".
func RecipientLabel(is Issue) string {
	recipients := RecipientsOf(is)
	names := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		names = append(names, recipient.Name)
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// Disposition is what the desk was told about a shortfall.
type Disposition string

const (
	ExpectedBack Disposition = "expected"    // "Still expected back"
	WontComeBack Disposition = "wont_return" // "Won't come back - broken or lost"
)

// Allocation puts part of a return against one issue line.
type Allocation struct {
	IssueID  string `json:"issueId"`
	Quantity int    `json:"quantity"` // >= 1
}

// Return is one handback of stock. A shortfall is an annotation on the record,
// never a stock movement: short items stay outstanding against the taker.
type Return struct {
	ID               string       `json:"id"` // "RET-0001"
	ProductID        string       `json:"productId"`
	Allocations      []Allocation `json:"allocations"` // >= 1 entry
	ReturnerName     string       `json:"returnerName"`
	ReturnerMobile   string       `json:"returnerMobile"`
	TakenBackBy      string       `json:"takenBackBy"` // on-duty staff name
	ReturnedAt       time.Time    `json:"returnedAt"`  // auto-filled, editable
	RecordedAt       time.Time    `json:"recordedAt"`
	ShortQuantity    int          `json:"shortQuantity"` // 0 when nothing was short
	ShortDisposition Disposition  `json:"shortDisposition,omitempty"`
	Remark           string       `json:"remark,omitempty"`
	Changes          []Change     `json:"changes,omitempty"`
	Deleted          *Deletion    `json:"deleted,omitempty"`
}

// Quantity is how many came back on this return. The allocations are the truth;
// the total is never stored.
func (r Return) Quantity() int {
	total := 0
	for _, a := range r.Allocations {
		total += a.Quantity
	}
	return total
}

// CleanName trims a free-text name and collapses internal runs of whitespace
// to a single space. Every name passes through it before storage.
func CleanName(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// FoldKey is the one key used wherever two names must be treated as the same name.
func FoldKey(s string) string {
	return strings.ToLower(CleanName(s))
}

// MobileKey keeps only the digits of a mobile number, so "98861 40023",
// "9886140023" and "98861-40023" are one number. Never used for display.
func MobileKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
