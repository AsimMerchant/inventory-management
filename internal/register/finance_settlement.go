package register

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// SupplierReturn is rented goods physically handed back to the supplier they
// came from. It moves stock and no money: a deposit coming back is its own
// incoming movement, recorded separately because it happens at another time.
type SupplierReturn struct {
	ID           string               `json:"id"` // SRN-0001
	DisposalID   string               `json:"disposalId"`
	PartyID      string               `json:"partyId"`
	Product      FinanceProductRef    `json:"product"`
	Sources      []DisposalAllocation `json:"sources"`
	ReturnedAt   time.Time            `json:"returnedAt"`
	Reference    string               `json:"reference,omitempty"`
	Remarks      string               `json:"remarks,omitempty"`
	RecordedAt   time.Time            `json:"recordedAt"`
	RecordedByID string               `json:"recordedById"`
	Changes      []FinanceChange      `json:"changes,omitempty"`
	Voided       *FinanceVoid         `json:"voided,omitempty"`
}

// StockSale is purchased goods sold on. Like a supplier return it moves stock
// and no money; the proceeds are their own incoming movements and may arrive
// in instalments or after the goods.
type StockSale struct {
	ID           string               `json:"id"` // SAL-0001
	DisposalID   string               `json:"disposalId"`
	BuyerPartyID string               `json:"buyerPartyId"`
	Product      FinanceProductRef    `json:"product"`
	Sources      []DisposalAllocation `json:"sources"`
	SoldAt       time.Time            `json:"soldAt"`
	Reference    string               `json:"reference,omitempty"`
	Remarks      string               `json:"remarks,omitempty"`
	RecordedAt   time.Time            `json:"recordedAt"`
	RecordedByID string               `json:"recordedById"`
	Changes      []FinanceChange      `json:"changes,omitempty"`
	Voided       *FinanceVoid         `json:"voided,omitempty"`
}

// FinanceSettlementRef links a money movement to the physical event it belongs
// with, without claiming the two happened at the same time or for equal value.
type FinanceSettlementRef struct {
	Kind string `json:"kind"` // supplier_return | sale
	ID   string `json:"id"`
}

// Live reports whether a settlement still counts.
func (s SupplierReturn) Live() bool { return s.Voided == nil }

// Live reports whether a sale still counts.
func (s StockSale) Live() bool { return s.Voided == nil }

// Quantity is the sum of a settlement's sources. It is never stored twice.
func allocated(sources []DisposalAllocation) int {
	total := 0
	for _, a := range sources {
		total += a.Quantity
	}
	return total
}

// SupplierReturnQuantity is how many went back to the supplier.
func (s SupplierReturn) Quantity() int { return allocated(s.Sources) }

// SaleQuantity is how many were sold.
func (s StockSale) Quantity() int { return allocated(s.Sources) }

// DisposalByID finds one public disposal.
func DisposalByID(r *Register, id string) (InventoryDisposal, bool) {
	for _, d := range r.Disposals {
		if d.ID == id {
			return d, true
		}
	}
	return InventoryDisposal{}, false
}

// ValidatePairing is the one invariant that spans both halves of the file: a
// live settlement in the vault and an active disposal in the public record are
// two views of one physical event, and neither may exist without the other.
// ValidateFinance cannot see the register, so this runs beside it.
func ValidatePairing(r *Register, f *FinanceData) error {
	if r == nil || f == nil {
		return nil
	}
	// Which disposal each settlement claims, and the shape it must have.
	claimed := map[string]bool{}

	check := func(kind, id, disposalID string, product FinanceProductRef,
		sources []DisposalAllocation, recordedAt time.Time, voided *FinanceVoid) error {

		if disposalID == "" {
			return fmt.Errorf("%s %s names no stock removal", kind, id)
		}
		if claimed[disposalID] {
			return fmt.Errorf("two settlements claim stock removal %s", disposalID)
		}
		claimed[disposalID] = true

		d, ok := DisposalByID(r, disposalID)
		if !ok {
			return fmt.Errorf("%s %s names an unknown stock removal", kind, id)
		}
		if d.ProductID != product.ProductID || d.Quantity != allocated(sources) {
			return fmt.Errorf("%s %s does not match its stock removal", kind, id)
		}
		if !d.RecordedAt.Equal(recordedAt) {
			return fmt.Errorf("%s %s was recorded at another time than its stock removal", kind, id)
		}
		if len(d.Sources) != len(sources) {
			return fmt.Errorf("%s %s does not match its stock removal", kind, id)
		}
		for i := range sources {
			if d.Sources[i] != sources[i] {
				return fmt.Errorf("%s %s does not match its stock removal", kind, id)
			}
		}

		switch {
		case voided != nil:
			// A voided settlement's stock came back, so its removal is off.
			if d.InactiveAt == nil || !d.InactiveAt.Equal(voided.At) {
				return fmt.Errorf("%s %s was voided but its stock removal still applies", kind, id)
			}
		case productTombstoned(r, product.ProductID):
			// The one legitimate way a live settlement has an inactive
			// removal: the whole product was deleted from the working
			// register afterwards. The protected history stays.
			at := productDeletedAt(r, product.ProductID)
			if at == nil || at.Before(recordedAt) {
				return fmt.Errorf("%s %s was recorded after its product was removed", kind, id)
			}
			if d.InactiveAt == nil || !d.InactiveAt.Equal(*at) {
				return fmt.Errorf("%s %s does not match its removed product", kind, id)
			}
		default:
			if d.InactiveAt != nil {
				return fmt.Errorf("%s %s is live but its stock removal does not apply", kind, id)
			}
		}
		return nil
	}

	for _, s := range f.SupplierReturns {
		if err := check("supplier return", s.ID, s.DisposalID, s.Product, s.Sources, s.RecordedAt, s.Voided); err != nil {
			return err
		}
	}
	for _, s := range f.Sales {
		if err := check("sale", s.ID, s.DisposalID, s.Product, s.Sources, s.RecordedAt, s.Voided); err != nil {
			return err
		}
	}

	// Every active disposal must have a settlement behind it. An orphan would
	// silently remove stock nobody can account for.
	for _, d := range r.Disposals {
		if d.InactiveAt == nil && !claimed[d.ID] {
			return fmt.Errorf("stock removal %s has no settlement behind it", d.ID)
		}
	}
	return nil
}

func productTombstoned(r *Register, productID string) bool {
	for _, p := range r.Products {
		if p.ID == productID {
			return p.Deleted != nil
		}
	}
	return false
}

func productDeletedAt(r *Register, productID string) *time.Time {
	for _, p := range r.Products {
		if p.ID == productID && p.Deleted != nil {
			at := p.Deleted.At
			return &at
		}
	}
	return nil
}

// SortedSettlements lists both kinds newest physical time first.
type SettlementRow struct {
	Kind        string // supplier_return | sale
	ID          string
	PartyID     string
	Product     FinanceProductRef
	Quantity    int
	At          time.Time
	Reference   string
	Remarks     string
	RecordedAt  time.Time
	RecordedBy  string
	Changes     []FinanceChange
	Voided      *FinanceVoid
	ProductGone bool
}

// SettlementRows is every physical exit, newest first, whichever kind it is.
func SettlementRows(r *Register, f *FinanceData) []SettlementRow {
	out := []SettlementRow{}
	for _, s := range f.SupplierReturns {
		out = append(out, SettlementRow{
			Kind: "supplier_return", ID: s.ID, PartyID: s.PartyID, Product: s.Product,
			Quantity: s.Quantity(), At: s.ReturnedAt, Reference: s.Reference,
			Remarks: s.Remarks, RecordedAt: s.RecordedAt, RecordedBy: s.RecordedByID,
			Changes: s.Changes, Voided: s.Voided,
			ProductGone: productTombstoned(r, s.Product.ProductID),
		})
	}
	for _, s := range f.Sales {
		out = append(out, SettlementRow{
			Kind: "sale", ID: s.ID, PartyID: s.BuyerPartyID, Product: s.Product,
			Quantity: s.Quantity(), At: s.SoldAt, Reference: s.Reference,
			Remarks: s.Remarks, RecordedAt: s.RecordedAt, RecordedBy: s.RecordedByID,
			Changes: s.Changes, Voided: s.Voided,
			ProductGone: productTombstoned(r, s.Product.ProductID),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// PartyAliases is every spelling that has ever meant this party: its current
// value, every wording it was corrected from, and the same for every value
// merged into it. An inward typed before a rename must still match.
func PartyAliases(f *FinanceData, partyID string) map[string]bool {
	out := map[string]bool{}
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if depth > len(f.ReusableValues)+1 {
			return
		}
		v, ok := FinanceValueByID(f, id)
		if !ok {
			return
		}
		if key := FoldKey(v.Value); key != "" {
			out[key] = true
		}
		for _, c := range v.Changes {
			if c.Field == "value" {
				if key := FoldKey(c.From); key != "" {
					out[key] = true
				}
			}
		}
		// Anything merged into this one carried its own history here.
		for _, other := range f.ReusableValues {
			if other.MergedIntoID == id {
				walk(other.ID, depth+1)
			}
		}
	}
	walk(resolvedPartyID(f, partyID), 0)
	return out
}

func resolvedPartyID(f *FinanceData, id string) string {
	if v, ok := ResolveFinanceValue(f, id); ok {
		return v.ID
	}
	return id
}

// eligibleInwards are the live inwards a settlement of this kind may draw on,
// oldest first: received date, then when it was typed, then id.
func eligibleInwards(r *Register, f *FinanceData, productID string, basis Basis, aliases map[string]bool) []Inward {
	out := []Inward{}
	for _, in := range LiveInwards(r) {
		if in.ProductID != productID || in.Basis != basis {
			continue
		}
		if aliases != nil {
			key := FoldKey(in.Supplier)
			// A blank supplier belongs to nobody and can never be returned.
			if key == "" || !aliases[key] {
				continue
			}
		}
		out = append(out, in)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ReceivedOn != out[j].ReceivedOn {
			return out[i].ReceivedOn < out[j].ReceivedOn
		}
		if !out[i].RecordedAt.Equal(out[j].RecordedAt) {
			return out[i].RecordedAt.Before(out[j].RecordedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// allocatedFromInward is how much of one inward live settlements already took,
// ignoring the settlement being edited.
func allocatedFromInward(f *FinanceData, inwardID, exceptKind, exceptID string) int {
	total := 0
	for _, s := range f.SupplierReturns {
		if !s.Live() || (exceptKind == "supplier_return" && s.ID == exceptID) {
			continue
		}
		for _, a := range s.Sources {
			if a.InwardID == inwardID {
				total += a.Quantity
			}
		}
	}
	for _, s := range f.Sales {
		if !s.Live() || (exceptKind == "sale" && s.ID == exceptID) {
			continue
		}
		for _, a := range s.Sources {
			if a.InwardID == inwardID {
				total += a.Quantity
			}
		}
	}
	return total
}

// AllocatedFromInward is how much of one inward live settlements have taken.
// An inward correction may not drop below this.
func AllocatedFromInward(f *FinanceData, inwardID string) int {
	return allocatedFromInward(f, inwardID, "", "")
}

// remaining is what is still drawable from each eligible inward, in order.
func remaining(r *Register, f *FinanceData, productID string, basis Basis,
	aliases map[string]bool, exceptKind, exceptID string) []DisposalAllocation {

	out := []DisposalAllocation{}
	for _, in := range eligibleInwards(r, f, productID, basis, aliases) {
		left := in.Quantity - allocatedFromInward(f, in.ID, exceptKind, exceptID)
		if left > 0 {
			out = append(out, DisposalAllocation{InwardID: in.ID, Quantity: left})
		}
	}
	return out
}

// onHandExcluding is the stock in the store ignoring one settlement's own
// current removal, which is what a correction has to measure against.
func onHandExcluding(r *Register, f *FinanceData, productID, exceptKind, exceptID string) int {
	skip := ""
	for _, s := range f.SupplierReturns {
		if exceptKind == "supplier_return" && s.ID == exceptID {
			skip = s.DisposalID
		}
	}
	for _, s := range f.Sales {
		if exceptKind == "sale" && s.ID == exceptID {
			skip = s.DisposalID
		}
	}
	total := CameIn(r, productID) - OutWithPeople(r, productID)
	for _, d := range LiveDisposals(r) {
		if d.ProductID == productID && d.ID != skip {
			total -= d.Quantity
		}
	}
	return total
}

func sumAllocations(rows []DisposalAllocation) int { return allocated(rows) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SupplierReturnAvailable is the most rented stock of this product that may
// leave the store now: the smaller of what is physically here and what came in
// on rent and has not gone back.
//
// It deliberately does not depend on who is receiving it. Goods do not always
// go straight back to the supplier who sent them — they are handed to a
// transporter, or to whoever is doing the rounds — and the register has no
// business refusing that. The party on a return records who took them.
func SupplierReturnAvailable(r *Register, f *FinanceData, partyID, productID string) int {
	return supplierReturnAvailable(r, f, partyID, productID, "", "")
}

func supplierReturnAvailable(r *Register, f *FinanceData, _, productID, exceptKind, exceptID string) int {
	rented := sumAllocations(remaining(r, f, productID, Rent, nil, exceptKind, exceptID))
	stock := onHandExcluding(r, f, productID, exceptKind, exceptID)
	return maxZero(minInt(stock, rented))
}

// SupplierSentRented is how much of this product came in on rent from one
// party and has not gone back yet, by ID or by the name typed on a screen. It
// exists so a long product list can be narrowed to one supplier's goods on
// request. It limits nothing: what may leave the store is SupplierReturnAvailable.
func SupplierSentRented(r *Register, f *FinanceData, party, productID string) int {
	aliases := map[string]bool{}
	if v, ok := ResolveFinanceValue(f, party); ok {
		aliases = PartyAliases(f, v.ID)
	} else if v, ok := FindFinanceValueByText(f, FinanceParty, party); ok {
		aliases = PartyAliases(f, v.ID)
	} else if key := FoldKey(party); key != "" {
		aliases = map[string]bool{key: true}
	}
	if len(aliases) == 0 {
		return 0
	}
	return maxZero(sumAllocations(remaining(r, f, productID, Rent, aliases, "", "")))
}

// SupplierReturnAvailableByName is the same number for a party typed on the
// screen who is not on the protected list yet. It is the same for everybody,
// so the name only has to be non-empty.
func SupplierReturnAvailableByName(r *Register, f *FinanceData, name, productID string) int {
	if FoldKey(name) == "" {
		return 0
	}
	return supplierReturnAvailable(r, f, "", productID, "", "")
}

// PurchasedAvailableToSell is the most that may be sold now: the smaller of
// what is in the store and what was bought and not yet sold.
func PurchasedAvailableToSell(r *Register, f *FinanceData, productID string) int {
	return purchasedAvailableToSell(r, f, productID, "", "")
}

func purchasedAvailableToSell(r *Register, f *FinanceData, productID, exceptKind, exceptID string) int {
	purchased := sumAllocations(remaining(r, f, productID, Purchase, nil, exceptKind, exceptID))
	stock := onHandExcluding(r, f, productID, exceptKind, exceptID)
	return maxZero(minInt(stock, purchased))
}

func maxZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// ErrNotEnoughStock is returned when a settlement asks for more than allowed.
var ErrNotEnoughStock = errors.New("not enough stock")

// AllocateSupplierReturn spreads a return across the rented receipts for this
// product, oldest first, whoever they came from. Stock is pooled: the program
// cannot know which physical chair is which, and makes no stronger claim.
func AllocateSupplierReturn(r *Register, f *FinanceData, partyID, productID string, quantity int) ([]DisposalAllocation, error) {
	return allocateSupplierReturn(r, f, partyID, productID, quantity, "", "")
}

func allocateSupplierReturn(r *Register, f *FinanceData, partyID, productID string, quantity int, exceptKind, exceptID string) ([]DisposalAllocation, error) {
	if quantity < 1 {
		return nil, ErrNotEnoughStock
	}
	if quantity > supplierReturnAvailable(r, f, partyID, productID, exceptKind, exceptID) {
		return nil, ErrNotEnoughStock
	}
	return take(remaining(r, f, productID, Rent, nil, exceptKind, exceptID), quantity), nil
}

// AllocateStockSale spreads a sale across purchased receipts, oldest first,
// whoever they came from.
func AllocateStockSale(r *Register, f *FinanceData, productID string, quantity int) ([]DisposalAllocation, error) {
	return allocateStockSale(r, f, productID, quantity, "", "")
}

func allocateStockSale(r *Register, f *FinanceData, productID string, quantity int, exceptKind, exceptID string) ([]DisposalAllocation, error) {
	if quantity < 1 {
		return nil, ErrNotEnoughStock
	}
	if quantity > purchasedAvailableToSell(r, f, productID, exceptKind, exceptID) {
		return nil, ErrNotEnoughStock
	}
	return take(remaining(r, f, productID, Purchase, nil, exceptKind, exceptID), quantity), nil
}

func take(available []DisposalAllocation, quantity int) []DisposalAllocation {
	out := []DisposalAllocation{}
	for _, a := range available {
		if quantity == 0 {
			break
		}
		n := minInt(a.Quantity, quantity)
		out = append(out, DisposalAllocation{InwardID: a.InwardID, Quantity: n})
		quantity -= n
	}
	return out
}

// SupplierObligation is what one supplier actually sent on rent and how much
// of it is still here. Orders and deposits do not appear: this counts goods.
type SupplierObligation struct {
	PartyID     string
	PartyName   string
	ProductID   string
	ProductName string
	Received    int
	Returned    int
	Remaining   int
}

// SupplierObligations groups live rented receipts by supplier and product. A
// supplier nobody has yet added to the protected list still gets a row, under
// the name the inward was typed with.
func SupplierObligations(r *Register, f *FinanceData) []SupplierObligation {
	type key struct{ party, product string }
	byKey := map[key]*SupplierObligation{}

	// Which folded supplier name belongs to which known party.
	owner := map[string]FinanceReusableValue{}
	for _, v := range LiveFinanceValues(f, FinanceParty) {
		for alias := range PartyAliases(f, v.ID) {
			owner[alias] = v
		}
	}

	for _, in := range LiveInwards(r) {
		if in.Basis != Rent {
			continue
		}
		fold := FoldKey(in.Supplier)
		if fold == "" {
			continue
		}
		partyID, partyName := "", CleanName(in.Supplier)
		if v, ok := owner[fold]; ok {
			partyID, partyName = v.ID, v.Value
		}
		k := key{fold, in.ProductID}
		row := byKey[k]
		if row == nil {
			name := ""
			if p, ok := ProductByID(r, in.ProductID); ok {
				name = p.Name
			}
			row = &SupplierObligation{
				PartyID: partyID, PartyName: partyName,
				ProductID: in.ProductID, ProductName: name,
			}
			byKey[k] = row
		}
		row.Received += in.Quantity
	}

	for _, s := range f.SupplierReturns {
		if !s.Live() {
			continue
		}
		v, ok := ResolveFinanceValue(f, s.PartyID)
		if !ok {
			continue
		}
		k := key{FoldKey(v.Value), s.Product.ProductID}
		if row := byKey[k]; row != nil {
			row.Returned += s.Quantity()
		}
	}

	out := []SupplierObligation{}
	for _, row := range byKey {
		row.Remaining = row.Received - row.Returned
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PartyName != out[j].PartyName {
			return out[i].PartyName < out[j].PartyName
		}
		return out[i].ProductName < out[j].ProductName
	})
	return out
}

// ReallocateSupplierReturn is AllocateSupplierReturn for a correction: it
// measures availability as if this settlement's own current removal were not
// there, so reducing a return never refuses itself.
func ReallocateSupplierReturn(r *Register, f *FinanceData, partyID, productID string, quantity int, exceptID string) ([]DisposalAllocation, error) {
	return allocateSupplierReturn(r, f, partyID, productID, quantity, "supplier_return", exceptID)
}

// ReallocateStockSale is AllocateStockSale for a correction.
func ReallocateStockSale(r *Register, f *FinanceData, productID string, quantity int, exceptID string) ([]DisposalAllocation, error) {
	return allocateStockSale(r, f, productID, quantity, "sale", exceptID)
}

// SupplierReturnAvailableExcluding is what a correction to this return may
// grow to, its own current removal set aside.
func SupplierReturnAvailableExcluding(r *Register, f *FinanceData, partyID, productID, exceptID string) int {
	return supplierReturnAvailable(r, f, partyID, productID, "supplier_return", exceptID)
}

// PurchasedAvailableToSellExcluding is the same for a sale correction.
func PurchasedAvailableToSellExcluding(r *Register, f *FinanceData, productID, exceptID string) int {
	return purchasedAvailableToSell(r, f, productID, "sale", exceptID)
}
