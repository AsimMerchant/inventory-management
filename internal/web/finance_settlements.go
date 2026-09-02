package web

import "net/http"

// The two physical end-of-event exits are spec 20. Their routes exist now so
// the dashboard never links to a dead page; the forms arrive with that spec.
func (s *Server) financeSupplierReturnNew(w http.ResponseWriter, r *http.Request) {
	s.financePage(w, r, "Return rented goods", "finance-soon.html", struct{ What string }{"Return rented goods"})
}

func (s *Server) financeSaleNew(w http.ResponseWriter, r *http.Request) {
	s.financePage(w, r, "Record a sale", "finance-soon.html", struct{ What string }{"Record a sale"})
}

func (s *Server) financeSettlements(w http.ResponseWriter, r *http.Request) {
	s.financePage(w, r, "Stock returned or sold", "finance-soon.html", struct{ What string }{"Stock returned or sold"})
}
