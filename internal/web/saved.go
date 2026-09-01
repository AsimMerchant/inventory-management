package web

import (
	"strconv"
	"strings"

	"storeregister/internal/register"
)

// savedBanners is the confirmation shown on /stock after an entry is saved.
// The sentences belong to specs 07, 08 and 09; /stock only draws them. A short
// return says two things: what came back, and what is still out with whom.
func savedBanners(reg *register.Register, id string) []banner {
	names := map[string]string{}
	for _, p := range reg.Products {
		names[p.ID] = p.Name
	}
	onHand := func(productID string) string {
		return names[productID] + ": " + strconv.Itoa(register.OnHand(reg, productID)) + " on hand."
	}

	switch {
	case strings.HasPrefix(id, "INW-"):
		for _, in := range register.LiveInwards(reg) {
			if in.ID == id {
				name := names[in.ProductID]
				return []banner{{"ok", "Added " + strconv.Itoa(in.Quantity) + " " +
					productWord(name) + ". " + onHand(in.ProductID)}}
			}
		}
	case strings.HasPrefix(id, "ISS-"):
		for _, is := range register.LiveIssues(reg) {
			if is.ID == id {
				name := names[is.ProductID]
				return []banner{{"ok", "Gave " + strconv.Itoa(is.Quantity) + " " +
					productWord(name) + " to " + register.RecipientLabel(is) + ". " + onHand(is.ProductID)}}
			}
		}
	case strings.HasPrefix(id, "RET-"):
		for _, re := range register.LiveReturns(reg) {
			if re.ID != id {
				continue
			}
			name := names[re.ProductID]
			out := []banner{{"ok", "Took back " + strconv.Itoa(re.Quantity()) + " " +
				productWord(name) + ". " + onHand(re.ProductID)}}
			// The shortfall is always against the person the stock was issued
			// to, never the person who handed it back.
			if re.ShortQuantity > 0 {
				taker := register.TakerOf(reg, re.Allocations)
				out = append(out, banner{"warn", taker + " still has " +
					strconv.Itoa(re.ShortQuantity) + " " + productWord(name) + "."})
			}
			return out
		}
	}
	return nil
}
