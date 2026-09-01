package register

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

var ErrProductDeleted = errors.New("product was deleted")

type ProductImpact struct {
	ProductID     string
	ProductName   string
	InwardEntries int
	IssueEntries  int
	ReturnEntries int
	CurrentlyOut  int
	Version       string
}

func ProductByID(r *Register, productID string) (Product, bool) {
	for _, p := range r.Products {
		if p.ID == productID && p.Deleted == nil {
			return p, true
		}
	}
	return Product{}, false
}

func productExists(r *Register, productID string) bool {
	_, ok := ProductByID(r, productID)
	return ok
}

func ProductDeletionImpact(r *Register, productID string) (ProductImpact, bool) {
	p, ok := ProductByID(r, productID)
	if !ok {
		return ProductImpact{}, false
	}
	impact := ProductImpact{ProductID: p.ID, ProductName: p.Name, CurrentlyOut: OutWithPeople(r, p.ID)}
	var inwards []Inward
	var issues []Issue
	var returns []Return
	for _, in := range r.Inwards {
		if in.ProductID == p.ID {
			inwards = append(inwards, in)
			if in.Deleted == nil {
				impact.InwardEntries++
			}
		}
	}
	for _, is := range r.Issues {
		if is.ProductID == p.ID {
			issues = append(issues, is)
			if is.Deleted == nil {
				impact.IssueEntries++
			}
		}
	}
	for _, re := range r.Returns {
		if re.ProductID == p.ID {
			returns = append(returns, re)
			if re.Deleted == nil {
				impact.ReturnEntries++
			}
		}
	}
	sort.Slice(inwards, func(i, j int) bool { return inwards[i].ID < inwards[j].ID })
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	sort.Slice(returns, func(i, j int) bool { return returns[i].ID < returns[j].ID })
	projection := struct {
		Product Product
		Inwards []Inward
		Issues  []Issue
		Returns []Return
	}{p, inwards, issues, returns}
	b, _ := json.Marshal(projection)
	sum := sha256.Sum256(b)
	impact.Version = hex.EncodeToString(sum[:])
	return impact, true
}

func RenameProduct(r *Register, productID, newName, by string, at time.Time) error {
	newName = CleanName(newName)
	for i := range r.Products {
		p := &r.Products[i]
		if p.ID != productID {
			continue
		}
		if p.Deleted != nil {
			return ErrProductDeleted
		}
		if p.Name == newName {
			return nil
		}
		old := p.Name
		p.Name = newName
		p.Changes = append(p.Changes, Change{At: at, By: by, Field: "productName", Label: "Product name", From: old, To: newName})
		return nil
	}
	return ErrUnknownProduct
}

func DeleteProductCascade(r *Register, productID, by string, at time.Time, reason string) error {
	reason = CleanName(reason)
	if reason == "" {
		return errors.New("deletion reason is required")
	}
	found := false
	d := Deletion{At: at, By: by, Reason: reason}
	for i := range r.Products {
		if r.Products[i].ID == productID {
			if r.Products[i].Deleted != nil {
				return ErrProductDeleted
			}
			r.Products[i].Deleted = &d
			found = true
		}
	}
	if !found {
		return ErrUnknownProduct
	}
	for i := range r.Inwards {
		if r.Inwards[i].ProductID == productID && r.Inwards[i].Deleted == nil {
			x := d
			r.Inwards[i].Deleted = &x
		}
	}
	for i := range r.Issues {
		if r.Issues[i].ProductID == productID && r.Issues[i].Deleted == nil {
			x := d
			r.Issues[i].Deleted = &x
		}
	}
	for i := range r.Returns {
		if r.Returns[i].ProductID == productID && r.Returns[i].Deleted == nil {
			x := d
			r.Returns[i].Deleted = &x
		}
	}
	if len(Validate(r)) != 0 {
		return errors.New("product deletion left an invalid register")
	}
	return nil
}
