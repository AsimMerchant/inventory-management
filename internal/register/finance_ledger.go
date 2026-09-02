package register

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// MoneyDirection says which way the money went. There is no third value: an
// adjustment that settles value without cash is still one direction or the
// other, and says so in its purpose and remarks.
type MoneyDirection string

const (
	MoneyOut MoneyDirection = "out"
	MoneyIn  MoneyDirection = "in"
)

// FinanceProductRef freezes what a transaction covered. The name is the name at
// the time; a later rename, tombstone or reuse of the same spelling never
// rewrites it.
type FinanceProductRef struct {
	ProductID   string `json:"productId"`
	ProductName string `json:"productName"`
}

// FinanceVoid marks an entry that should never have been recorded. It never
// removes the row: to reverse real money, record the opposite direction.
type FinanceVoid struct {
	At          time.Time `json:"at"`
	ByAccountID string    `json:"byAccountId"`
	ByName      string    `json:"byName"`
	ByMobile    string    `json:"byMobile"`
	Reason      string    `json:"reason"`
}

// MoneyMovement is one payment out or one receipt in. Its timing is its own:
// it may come before the goods, long after them, or in instalments.
type MoneyMovement struct {
	ID           string              `json:"id"` // MOV-0001
	Direction    MoneyDirection      `json:"direction"`
	AmountPaise  int64               `json:"amountPaise"`
	OccurredAt   time.Time           `json:"occurredAt"`
	PartyID      string              `json:"partyId"`
	OrderID      string              `json:"orderId,omitempty"`
	OrderLineIDs []string            `json:"orderLineIds,omitempty"`
	Products     []FinanceProductRef `json:"products,omitempty"`
	PurposeID    string              `json:"purposeId"`
	ModeID       string              `json:"modeId"`
	Reference    string              `json:"reference,omitempty"`
	Remarks      string              `json:"remarks,omitempty"`
	RecordedAt   time.Time           `json:"recordedAt"`
	RecordedByID string              `json:"recordedById"`
	Changes      []FinanceChange     `json:"changes,omitempty"`
	Voided       *FinanceVoid        `json:"voided,omitempty"`
}

// Live reports whether this movement counts towards any total.
func (m MoneyMovement) Live() bool { return m.Voided == nil }

// ErrMoneyOverflow is the refusal when a total would not fit in int64 paise.
// Money that silently wraps round is worse than money that is refused.
var ErrMoneyOverflow = errors.New("That amount is too large to total safely.")

// AddPaise adds two amounts and refuses rather than wrapping.
func AddPaise(a, b int64) (int64, error) {
	if b > 0 && a > (1<<63-1)-b {
		return 0, ErrMoneyOverflow
	}
	if b < 0 && a < -(1<<63)-b {
		return 0, ErrMoneyOverflow
	}
	return a + b, nil
}

// SubPaise subtracts and refuses rather than wrapping.
func SubPaise(a, b int64) (int64, error) {
	if b == -(1 << 63) {
		return 0, ErrMoneyOverflow
	}
	return AddPaise(a, -b)
}

// MoneyTotals is what the dashboard and an order summary both show.
type MoneyTotals struct {
	PaidPaise     int64
	ReceivedPaise int64
	NetPaise      int64
}

// TotalMoney sums every live movement the keep function accepts. Passing a
// function that always accepts gives the whole-ledger totals.
func TotalMoney(f *FinanceData, keep func(MoneyMovement) bool) (MoneyTotals, error) {
	var out MoneyTotals
	var err error
	for _, m := range f.Movements {
		if !m.Live() || (keep != nil && !keep(m)) {
			continue
		}
		switch m.Direction {
		case MoneyOut:
			out.PaidPaise, err = AddPaise(out.PaidPaise, m.AmountPaise)
		case MoneyIn:
			out.ReceivedPaise, err = AddPaise(out.ReceivedPaise, m.AmountPaise)
		}
		if err != nil {
			return MoneyTotals{}, err
		}
	}
	out.NetPaise, err = SubPaise(out.PaidPaise, out.ReceivedPaise)
	if err != nil {
		return MoneyTotals{}, err
	}
	return out, nil
}

// OrderTotals sums the live movements linked to one order.
func OrderTotals(f *FinanceData, orderID string) (MoneyTotals, error) {
	return TotalMoney(f, func(m MoneyMovement) bool { return m.OrderID == orderID })
}

// MovementByID finds one movement whether or not it is voided.
func MovementByID(f *FinanceData, id string) (MoneyMovement, bool) {
	for _, m := range f.Movements {
		if m.ID == id {
			return m, true
		}
	}
	return MoneyMovement{}, false
}

// SortedMovements is the journal's screen order: what happened most recently
// first, and for two things at the same moment the later entry first.
func SortedMovements(rows []MoneyMovement) []MoneyMovement {
	out := append([]MoneyMovement{}, rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.After(out[j].OccurredAt)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// AscendingMovements is the printed order: oldest first, so a page of paper
// reads down the day the way a ledger book does.
func AscendingMovements(rows []MoneyMovement) []MoneyMovement {
	out := append([]MoneyMovement{}, rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.Before(out[j].OccurredAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// SortedAudit is the financial activity order: when it was actually done,
// most recent first.
func SortedAudit(f *FinanceData) []FinanceAuditEvent {
	out := append([]FinanceAuditEvent{}, f.Audit...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// JournalRefusal is the one thing a person is told when a filter will not parse.
const JournalRefusal = "Choose a valid date or date range."

// JournalFilter is the journal's date window, already parsed and checked.
// Zero start and end mean every date.
type JournalFilter struct {
	Start, End time.Time
	Every      bool
	Label      string
}

// Keep reports whether a movement falls inside the window. Both ends are
// inclusive, and the comparison is on when the money moved, not when somebody
// typed it in: a payment entered late still prints on the day it happened.
func (f JournalFilter) Keep(m MoneyMovement) bool {
	if f.Every {
		return true
	}
	return !m.OccurredAt.Before(f.Start) && !m.OccurredAt.After(f.End)
}

// ParseJournalFilter reads the three filter shapes. An exact time range wins
// over a single day, which wins over a date range, so a person narrowing down
// to minutes is never quietly given the whole day instead.
func ParseJournalFilter(day, from, to, fromTime, toTime string, loc *time.Location) (JournalFilter, error) {
	day, from, to = strings.TrimSpace(day), strings.TrimSpace(from), strings.TrimSpace(to)
	fromTime, toTime = strings.TrimSpace(fromTime), strings.TrimSpace(toTime)

	if fromTime != "" || toTime != "" {
		if fromTime == "" || toTime == "" {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		start, err := time.ParseInLocation("2006-01-02T15:04", fromTime, loc)
		if err != nil {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		end, err := time.ParseInLocation("2006-01-02T15:04", toTime, loc)
		if err != nil {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		if end.Before(start) {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		// The minute typed is included whole, so 23:39 keeps a 23:39 entry.
		end = end.Add(time.Minute - time.Nanosecond)
		return JournalFilter{Start: start, End: end,
			Label: start.Format("2 January 2006 · 3:04 pm") + " to " + toTime[11:] + " on " + end.Format("2 January 2006")}, nil
	}

	if day != "" {
		start, err := time.ParseInLocation("2006-01-02", day, loc)
		if err != nil {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		return JournalFilter{Start: start, End: start.AddDate(0, 0, 1).Add(-time.Nanosecond),
			Label: start.Format("2 January 2006")}, nil
	}

	if from != "" || to != "" {
		if from == "" || to == "" {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		start, err := time.ParseInLocation("2006-01-02", from, loc)
		if err != nil {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		end, err := time.ParseInLocation("2006-01-02", to, loc)
		if err != nil {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		if end.Before(start) {
			return JournalFilter{}, errors.New(JournalRefusal)
		}
		return JournalFilter{Start: start, End: end.AddDate(0, 0, 1).Add(-time.Nanosecond),
			Label: start.Format("2 January 2006") + " to " + end.Format("2 January 2006")}, nil
	}

	return JournalFilter{Every: true, Label: "Every date"}, nil
}

// DirectionText is how a direction is written wherever a person reads it.
func DirectionText(d MoneyDirection) string {
	if d == MoneyIn {
		return "Money received"
	}
	return "Money paid"
}
