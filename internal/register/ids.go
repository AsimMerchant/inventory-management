package register

import (
	"fmt"
	"strconv"
	"strings"
)

// NextID returns prefix + "-" + a zero-padded number one higher than the highest
// existing suffix with that prefix. Padding widens past 9999 ("INW-10000").
// Identifiers are never typed or seen by the person at the desk.
func (r *Register) NextID(prefix string) string {
	highest := 0
	for _, id := range r.idsFor(prefix) {
		n, ok := suffixOf(id, prefix)
		if ok && n > highest {
			highest = n
		}
	}
	return fmt.Sprintf("%s-%04d", prefix, highest+1)
}

func (r *Register) idsFor(prefix string) []string {
	var ids []string
	switch prefix {
	case "PRD":
		for _, p := range r.Products {
			ids = append(ids, p.ID)
		}
	case "STF":
		for _, s := range r.Staff {
			ids = append(ids, s.ID)
		}
	case "INW":
		for _, in := range r.Inwards {
			ids = append(ids, in.ID)
		}
	case "ISS":
		for _, is := range r.Issues {
			ids = append(ids, is.ID)
		}
	case "RET":
		for _, re := range r.Returns {
			ids = append(ids, re.ID)
		}
	case "DSP":
		for _, d := range r.Disposals {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

func suffixOf(id, prefix string) (int, bool) {
	rest, found := strings.CutPrefix(id, prefix+"-")
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}
