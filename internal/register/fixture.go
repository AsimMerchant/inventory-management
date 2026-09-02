package register

import "time"

// IST is the fixture's timezone. It is a fixed zone and never time.LoadLocation:
// a CGO_ENABLED=0 GOOS=windows binary carries no tzdata unless time/tzdata is
// imported, and a fixed zone keeps the timestamp tests deterministic wherever
// the developer's machine happens to be.
var IST = time.FixedZone("IST", 5*3600+30*60)

// MustTime parses an RFC 3339 timestamp and panics if it cannot. For fixtures
// and tests only.
func MustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("register: bad fixture time " + s + ": " + err.Error())
	}
	return t
}

func at(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, IST)
}

// WalkthroughT0 is the register as the walkthrough's home screen shows it:
// 3 September 2026, 10:00 am. Every other spec's test cases are written
// against this register and the timepoints built from it.
func WalkthroughT0() *Register {
	created := at(2026, time.September, 1, 8, 0)
	shiftStarted := at(2026, time.September, 3, 8, 0)

	suresh := "Suresh Kumar"
	sureshMobile := "98450 22117"
	anita := "Anita Rao"
	anitaMobile := "99001 34562"
	imran := "Imran Sheikh"
	imranMobile := "90080 77213"

	return &Register{
		SchemaVersion:  SchemaVersion,
		OnDutyStaffID:  "STF-0001",
		ShiftStartedAt: &shiftStarted,
		Products: []Product{
			{ID: "PRD-0001", Name: "Chairs", CreatedAt: created, CreatedBy: suresh},
			{ID: "PRD-0002", Name: "Round tables", CreatedAt: created, CreatedBy: suresh},
			{ID: "PRD-0003", Name: "Water drums (20L)", CreatedAt: created, CreatedBy: suresh},
			{ID: "PRD-0004", Name: "Extension boards", CreatedAt: created, CreatedBy: suresh},
			{ID: "PRD-0005", Name: "Charcoal sacks", CreatedAt: created, CreatedBy: suresh},
		},
		Staff: []Staff{
			// Nobody was on duty when the first person was added, so
			// CreatedBy is empty. That is a real value, not a gap.
			{ID: "STF-0001", Name: suresh, Mobile: sureshMobile, CreatedAt: at(2026, time.September, 1, 7, 30), CreatedBy: ""},
			{ID: "STF-0002", Name: anita, Mobile: anitaMobile, CreatedAt: at(2026, time.September, 1, 7, 35), CreatedBy: suresh},
			{ID: "STF-0003", Name: imran, Mobile: imranMobile, CreatedAt: at(2026, time.September, 1, 7, 40), CreatedBy: suresh},
		},
		Inwards: []Inward{
			{
				ID: "INW-0001", ProductID: "PRD-0001", Quantity: 390,
				ReceivedOn: "2026-09-01", Basis: Rent,
				Supplier: "Sharma Tent House", ChallanNo: "STH/4390",
				ReceivedBy: suresh, RecordedAt: at(2026, time.September, 1, 9, 15), RecordedBy: suresh,
			},
			{
				ID: "INW-0002", ProductID: "PRD-0001", Quantity: 310,
				ReceivedOn: "2026-09-02", Basis: Purchase,
				ReceivedBy: suresh, RecordedAt: at(2026, time.September, 2, 8, 30), RecordedBy: suresh,
			},
			{
				ID: "INW-0003", ProductID: "PRD-0002", Quantity: 60,
				ReceivedOn: "2026-09-01", Basis: Rent,
				Supplier: "Sharma Tent House", ChallanNo: "STH/4390",
				ReceivedBy: suresh, RecordedAt: at(2026, time.September, 1, 9, 20), RecordedBy: suresh,
			},
			{
				ID: "INW-0004", ProductID: "PRD-0003", Quantity: 40,
				ReceivedOn: "2026-09-01", Basis: Purchase,
				ReceivedBy: anita, RecordedAt: at(2026, time.September, 1, 11, 0), RecordedBy: anita,
			},
			{
				ID: "INW-0005", ProductID: "PRD-0004", Quantity: 25,
				ReceivedOn: "2026-09-02", Basis: Rent,
				Supplier: "Gupta Electricals", ChallanNo: "GE/118",
				ReceivedBy: imran, RecordedAt: at(2026, time.September, 2, 9, 0), RecordedBy: imran,
			},
			{
				ID: "INW-0006", ProductID: "PRD-0005", Quantity: 12,
				ReceivedOn: "2026-09-02", Basis: Purchase,
				ReceivedBy: anita, RecordedAt: at(2026, time.September, 2, 10, 0), RecordedBy: anita,
			},
		},
		Issues: []Issue{
			issue("ISS-0001", "PRD-0001", 150, "Lakshmi Iyer", "Kitchen", "99860 11204",
				suresh, sureshMobile, at(2026, time.September, 1, 8, 30)),
			issue("ISS-0002", "PRD-0001", 120, "Joseph D'Cruz", "Stage & Sound", "90350 66471",
				imran, imranMobile, at(2026, time.September, 1, 9, 15)),
			issue("ISS-0003", "PRD-0001", 40, "Ravi Menon", "Catering", "98861 40023",
				suresh, sureshMobile, at(2026, time.September, 3, 9, 40)),
			issue("ISS-0004", "PRD-0002", 10, "Lakshmi Iyer", "Kitchen", "99860 11204",
				anita, anitaMobile, at(2026, time.September, 2, 9, 50)),
			issue("ISS-0005", "PRD-0002", 2, "Ravi Menon", "Catering", "98861 40023",
				suresh, sureshMobile, at(2026, time.September, 3, 9, 40)),
			issue("ISS-0006", "PRD-0003", 5, "Farida Begum", "Registration", "98455 30918",
				anita, anitaMobile, at(2026, time.September, 2, 12, 15)),
			issue("ISS-0007", "PRD-0004", 25, "Joseph D'Cruz", "Stage & Sound", "90350 66471",
				imran, imranMobile, at(2026, time.September, 1, 9, 45)),
		},
		Returns: []Return{},
		// Empty, like every other slice: the reader normalises a missing key
		// to this, so a fixture that left it nil would not compare equal to
		// the same register read back off disk.
		Disposals: []InventoryDisposal{},
	}
}

// issue builds a fixture issue whose RecordedAt equals its IssuedAt.
func issue(id, productID string, qty int, taker, dept, mobile, incharge, inchargeMobile string, issuedAt time.Time) Issue {
	return Issue{
		ID: id, ProductID: productID, Quantity: qty,
		TakerName: taker, TakerDepartment: dept, TakerMobile: mobile,
		PersonInchargeName: incharge, PersonInchargeMobile: inchargeMobile,
		IssuedAt: issuedAt, RecordedAt: issuedAt,
	}
}

// WalkthroughT1 is T0 plus INW-0007: 500 chairs from Sharma Tent House at 10:42.
func WalkthroughT1() *Register {
	r := WalkthroughT0()
	r.Inwards = append(r.Inwards, Inward{
		ID: "INW-0007", ProductID: "PRD-0001", Quantity: 500,
		ReceivedOn: "2026-09-03", Basis: Rent,
		Supplier: "Sharma Tent House", ChallanNo: "STH/4471",
		ReceivedBy: "Suresh Kumar", RecordedAt: at(2026, time.September, 3, 10, 42),
		RecordedBy: "Suresh Kumar",
	})
	return r
}

// WalkthroughT2 is T1 plus ISS-0008: 10 chairs to Ravi Menon at 14:18.
func WalkthroughT2() *Register {
	r := WalkthroughT1()
	r.Issues = append(r.Issues, issue("ISS-0008", "PRD-0001", 10,
		"Ravi Menon", "Catering", "98861 40023",
		"Anita Rao", "99001 34562", at(2026, time.September, 3, 14, 18)))
	return r
}

// WalkthroughT3 is T2 plus RET-0001: 45 chairs back from Ravi Menon at 18:05,
// 5 short and not coming back.
func WalkthroughT3() *Register {
	r := WalkthroughT2()
	returnedAt := at(2026, time.September, 3, 18, 5)
	r.Returns = append(r.Returns, Return{
		ID: "RET-0001", ProductID: "PRD-0001",
		Allocations: []Allocation{
			{IssueID: "ISS-0003", Quantity: 40},
			{IssueID: "ISS-0008", Quantity: 5},
		},
		ReturnerName: "Ravi Menon", ReturnerMobile: "98861 40023",
		TakenBackBy: "Imran Sheikh",
		ReturnedAt:  returnedAt, RecordedAt: returnedAt,
		ShortQuantity: 5, ShortDisposition: WontComeBack,
		Remark: "5 chairs broke during setup near the stage. Ravi informed.",
	})
	return r
}
