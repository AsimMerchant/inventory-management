package web

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"storeregister/internal/register"
)

// The four reading screens. Nothing here changes a record: the only controls
// are links into the three entry flows and into the correction screens. Every
// figure comes from internal/register; this file writes them down.

// ---------------------------------------------------------------------------
// Stock
// ---------------------------------------------------------------------------

type stockRow struct {
	ProductID string
	Name      string
	PillClass string // "pill rent" or "pill sale"
	PillWord  string // "Rent" or "Purchase"
	CameIn    int
	Out       int
	OnHand    int
	IssueHref string // "" when there is nothing left to give out
	FixHref   string
}

type stockData struct {
	Tiles register.Tiles
	Rows  []stockRow
}

func (s *Server) stockView(w http.ResponseWriter, r *http.Request) {
	p := s.page("Stock")
	p.Current = "/stock"

	var data stockData
	s.st.Read(func(reg *register.Register) {
		data.Tiles = register.TileCounts(reg, s.now())
		for _, row := range register.StockRows(reg) {
			out := stockRow{
				ProductID: row.ProductID, Name: row.Name,
				PillClass: "pill rent", PillWord: "Rent",
				CameIn: row.CameIn, Out: row.Out, OnHand: row.OnHand,
				FixHref: "/product/" + row.ProductID + "/edit",
			}
			if row.Basis != register.Rent {
				out.PillClass, out.PillWord = "pill sale", "Purchase"
			}
			if row.OnHand > 0 {
				out.IssueHref = "/issue/new?productId=" + row.ProductID
			}
			data.Rows = append(data.Rows, out)
		}
		if id := r.URL.Query().Get("saved"); id != "" {
			p.Banners = savedBanners(reg, id)
		}
		if id := r.URL.Query().Get("renamed"); id != "" {
			if prod, ok := register.ProductByID(reg, id); ok {
				p.add(&banner{"ok", "Renamed " + r.URL.Query().Get("old") + " to " + prod.Name + "."})
			}
		}
		if id := r.URL.Query().Get("productDeleted"); id != "" {
			for _, prod := range reg.Products {
				if prod.ID == id && prod.Deleted != nil {
					p.add(&banner{"ok", "Deleted " + prod.Name + " and all of its entries. The history is still in Who did what."})
				}
			}
		}
	})
	s.render(w, http.StatusOK, p, "stock.html", data)
}

// ---------------------------------------------------------------------------
// Out with people
// ---------------------------------------------------------------------------

type outLine struct {
	IssueID     string
	ProductName string
	Middle      string // "Issued 9:40 am by the staff member · 40 taken, 0 back"
	Out         int
	Aged        bool
	Shortfalls  []string
	Changes     []string
	FixHref     string
	ChallanNo   string
}

type personBlock struct {
	Name           string
	Department     string
	Mobile         string
	TotalOut       int
	ContextSummary string
	ReturnHref     string
	Lines          []outLine
}

type cameBackLine struct {
	At        time.Time // ReturnedAt, for the ordering; never drawn
	ReturnID  string
	Headline  string // "45 chairs from Ravi Menon · 6:05 pm · taken back by Imran Sheikh"
	Shortfall string
	Changes   []string
	FixHref   string
	Deleted   bool
	Deletion  string
}

type outData struct {
	People []personBlock
	Back   []cameBackLine
}

func (s *Server) outView(w http.ResponseWriter, r *http.Request) {
	p := s.page("Out with people")
	p.Current = "/out"

	var data outData
	s.st.Read(func(reg *register.Register) {
		data = outPage(reg, s.now())
		p.Banners = correctionBanners(reg, r)
	})
	s.render(w, http.StatusOK, p, "out.html", data)
}

// outPage lays out one block per person and, beneath them, every return.
func outPage(reg *register.Register, now time.Time) outData {
	var data outData
	cutoff := now.Add(-48 * time.Hour)
	changesOn := changeIndex(reg)

	holdings := register.JointHoldings(reg)
	jointMember := map[register.PersonID]bool{}
	for _, holding := range holdings {
		if len(holding.Recipients) > 1 {
			for _, recipient := range holding.Recipients {
				jointMember[register.PersonOf(recipient.Name, recipient.Mobile)] = true
			}
		}
	}
	for _, holding := range holdings {
		first := holding.Recipients[0]
		query := first.Mobile
		if query == "" {
			query = first.Name
		}
		context := ""
		if len(holding.Recipients) > 1 {
			context = " - holding together"
		} else if jointMember[register.PersonOf(first.Name, first.Mobile)] {
			context = " - holding alone"
		}
		block := personBlock{
			Name: holding.Label + context, Department: first.Department, Mobile: first.Mobile,
			TotalOut:   holding.TotalOut,
			ReturnHref: "/return/new?q=" + url.QueryEscape(query) + "&holdingIssueId=" + holding.AnchorIssueID,
		}
		if context != "" {
			block.Name, block.Department, block.Mobile = holding.Label, "", ""
			block.ContextSummary = holding.Label + context + " - " + strconv.Itoa(holding.TotalOut) + " out"
		}
		for _, l := range holding.Lines {
			block.Lines = append(block.Lines, outLine{
				IssueID:     l.IssueID,
				ProductName: l.ProductName,
				Middle: "Issued " + clock(l.IssuedAt) + " by " + l.IssuedBy + " · " +
					strconv.Itoa(l.Taken) + " taken, " + strconv.Itoa(l.Back) + " back",
				Out:        l.Out,
				Aged:       l.IssuedAt.Before(cutoff),
				Shortfalls: shortfallsAgainst(reg, l.IssueID),
				Changes:    changesOn[l.IssueID],
				FixHref:    "/entry/" + l.IssueID + "/edit",
				ChallanNo:  l.ChallanNo,
			})
		}
		data.People = append(data.People, block)
	}

	names := productNames(reg)
	for _, re := range reg.Returns {
		if _, ok := register.ProductByID(reg, re.ProductID); !ok {
			continue
		}
		line := cameBackLine{
			At: re.ReturnedAt, ReturnID: re.ID,
			Headline: strconv.Itoa(re.Quantity()) + " " + productWord(names[re.ProductID]) +
				" from " + re.ReturnerName + " · " + clock(re.ReturnedAt) +
				" · taken back by " + re.TakenBackBy,
			Shortfall: shortfallLine(re),
			Changes:   changesOn[re.ID],
			FixHref:   "/entry/" + re.ID + "/edit",
		}
		if re.Deleted != nil {
			line.Deleted, line.Deletion, line.FixHref = true, deletionLine(*re.Deleted), ""
		}
		data.Back = append(data.Back, line)
	}
	// Newest ReturnedAt first, ties broken by ID so the order never wobbles.
	sort.SliceStable(data.Back, func(i, j int) bool {
		a, b := data.Back[i], data.Back[j]
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		return a.ReturnID > b.ReturnID
	})
	return data
}

// shortfallsAgainst is every remark a return left against one issue line.
func shortfallsAgainst(reg *register.Register, issueID string) []string {
	var out []string
	for _, re := range register.LiveReturns(reg) {
		if re.ShortQuantity == 0 {
			continue
		}
		for _, a := range re.Allocations {
			if a.IssueID == issueID {
				out = append(out, shortfallLine(re))
			}
		}
	}
	return out
}

// shortfallLine is what the desk was told about the missing items, prefixed by
// what is going to happen to them. The same sentence is quoted on /log.
func shortfallLine(re register.Return) string {
	if re.ShortQuantity == 0 || re.Remark == "" {
		return ""
	}
	if re.ShortDisposition == register.WontComeBack {
		return "Won't come back: " + re.Remark
	}
	return "Still expected back: " + re.Remark
}

// changeIndex is every record's corrections in words, by record ID.
func changeIndex(reg *register.Register) map[string][]string {
	names := productNames(reg)
	out := map[string][]string{}
	for _, in := range reg.Inwards {
		out[in.ID] = changeLines(in.Changes, productWord(names[in.ProductID]))
	}
	for _, is := range reg.Issues {
		out[is.ID] = changeLines(is.Changes, productWord(names[is.ProductID]))
	}
	for _, re := range reg.Returns {
		out[re.ID] = changeLines(re.Changes, productWord(names[re.ProductID]))
	}
	return out
}

// ---------------------------------------------------------------------------
// Stuff came in
// ---------------------------------------------------------------------------

type inwardRow struct {
	At          time.Time // RecordedAt, for the ordering; never drawn
	ID          string
	ReceivedOn  string
	ProductName string
	Quantity    int
	PillClass   string
	PillWord    string
	Supplier    string
	ChallanNo   string
	ReceivedBy  string
	Changes     []string
	FixHref     string
	Deleted     bool
	Deletion    string
}

type inwardsData struct {
	Rows []inwardRow
}

func (s *Server) inwardsView(w http.ResponseWriter, r *http.Request) {
	p := s.page("Stuff came in")
	p.Current = "/inwards"

	var data inwardsData
	s.st.Read(func(reg *register.Register) {
		names := productNames(reg)
		changesOn := changeIndex(reg)
		for _, in := range reg.Inwards {
			if _, ok := register.ProductByID(reg, in.ProductID); !ok {
				continue
			}
			row := inwardRow{
				At: in.RecordedAt, ID: in.ID, ReceivedOn: in.ReceivedOn,
				ProductName: names[in.ProductID], Quantity: in.Quantity,
				PillClass: "pill rent", PillWord: "Rent",
				Supplier: in.Supplier, ChallanNo: in.ChallanNo, ReceivedBy: in.ReceivedBy,
				Changes: changesOn[in.ID], FixHref: "/entry/" + in.ID + "/edit",
			}
			if in.Basis != register.Rent {
				row.PillClass, row.PillWord = "pill sale", "Purchase"
			}
			if in.Deleted != nil {
				row.Deleted, row.Deletion, row.FixHref = true, deletionLine(*in.Deleted), ""
			}
			data.Rows = append(data.Rows, row)
		}
		// Newest RecordedAt first, ties broken by ID.
		sort.SliceStable(data.Rows, func(i, j int) bool {
			a, b := data.Rows[i], data.Rows[j]
			if !a.At.Equal(b.At) {
				return a.At.After(b.At)
			}
			return a.ID > b.ID
		})
		p.Banners = correctionBanners(reg, r)
	})
	s.render(w, http.StatusOK, p, "inwards.html", data)
}

// ---------------------------------------------------------------------------
// Suppliers
// ---------------------------------------------------------------------------

type supplierRow struct {
	Supplier     string
	Bought       bool // no supplier was recorded, so the cell reads "we bought it"
	ProductName  string
	CameIn       int
	PillClass    string
	PillWord     string
	WontComeBack string // "5 broken or lost", "" when none are
}

type suppliersData struct {
	OutRightNow int
	AnythingOut bool
	Rows        []supplierRow
}

func (s *Server) suppliersView(w http.ResponseWriter, r *http.Request) {
	p := s.page("Suppliers")
	p.Current = "/suppliers"

	var data suppliersData
	s.st.Read(func(reg *register.Register) {
		data.OutRightNow = register.TileCounts(reg, s.now()).OutRightNow
		data.AnythingOut = data.OutRightNow > 0
		for _, row := range register.SupplierRows(reg) {
			out := supplierRow{
				Supplier: row.Supplier, ProductName: row.ProductName, CameIn: row.CameIn,
				PillClass: "pill rent", PillWord: "Rent",
			}
			if !row.OnRent {
				out.PillClass, out.PillWord = "pill sale", "Purchase"
			}
			if row.Supplier == "" {
				out.Bought = true
			}
			if row.WontComeBack > 0 {
				out.WontComeBack = strconv.Itoa(row.WontComeBack) + " broken or lost"
			}
			data.Rows = append(data.Rows, out)
		}
	})
	s.render(w, http.StatusOK, p, "suppliers.html", data)
}

// correctionBanners is what /inwards and /out say when a correction lands on
// them. No confirmation is ever drawn on /suppliers: that page keeps no score.
func correctionBanners(reg *register.Register, r *http.Request) []banner {
	if id := r.URL.Query().Get("fixed"); id != "" {
		return fixedBanner(reg, id)
	}
	if id := r.URL.Query().Get("deleted"); id != "" {
		return deletedBanner(reg, id)
	}
	return nil
}
