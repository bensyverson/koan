package koan

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// Koan is one rendered draw. It is the JSONL wire shape: one self-contained
// object per draw, so a line cut out of a log reproduces without its
// neighbours.
type Koan struct {
	// Seed is the seed the run was drawn under.
	Seed uint64 `json:"seed"`
	// Index is the 0-based position of this draw in the run.
	Index int `json:"i"`
	// Template is how the template was named: its path, or "-" for stdin.
	Template string `json:"template"`
	// Slots maps each slot name to the value drawn for it.
	Slots map[string]string `json:"slots"`
	// Text is the rendered template.
	Text string `json:"text"`
}

// Request is everything a draw needs.
type Request struct {
	// Template is the parsed template. It must not be nil.
	Template *Template
	// Name is how the template is reported in Koan.Template: its path, or
	// "-" for stdin.
	Name string
	// Bags holds one bag per slot, keyed by slot name — from LoadBags(dir,
	// Template.Slots()), after Exclude when --used is in play.
	Bags map[string]*Bag
	// Seed seeds the stream. Two requests alike in every field draw alike.
	Seed uint64
	// Count is how many draws to make. Below 1 draws nothing.
	Count int
}

// Draw renders req.Count draws from one stream seeded (req.Seed, 0).
//
// Each draw pulls one value per slot, in the template's order of first
// appearance, so a draw consumes exactly len(Template.Slots()) random numbers
// and -n 5 begins with the three draws of -n 3 under the same seed. Within one
// run a bag never repeats a value: drawing a value removes every copy of it
// from that bag's remaining pool, so a duplicated line is a weight rather than
// a second chance. The caller's bags are never mutated.
//
// Both failures are found before anything is drawn, so a caller never sees a
// partial run: Draw returns koans or an error, never both. A slot with no bag
// in req.Bags is an *UnboundSlotError; a bag that cannot supply req.Count
// distinct values is an *ExhaustedError naming the first such bag in slot
// order.
//
// A req.Count below 1 returns (nil, nil): the CLI rejects it earlier as a
// usage error, and there is nothing for the library to object to.
func Draw(req Request) ([]Koan, error) {
	if req.Count < 1 {
		return nil, nil
	}

	slots := req.Template.Slots()
	for _, slot := range slots {
		if _, ok := req.Bags[slot]; !ok {
			return nil, &UnboundSlotError{Slot: slot}
		}
	}
	pools := make([][]string, len(slots))
	for i, slot := range slots {
		bag := req.Bags[slot]
		if available := distinctCount(bag.Values); available < req.Count {
			return nil, &ExhaustedError{
				Name:      bag.Name,
				Path:      bag.Path,
				Available: available,
				Needed:    req.Count,
			}
		}
		pools[i] = slices.Clone(bag.Values)
	}

	stream := rand.New(rand.NewPCG(req.Seed, 0))
	koans := make([]Koan, 0, req.Count)
	for i := range req.Count {
		values := make(map[string]string, len(slots))
		for s, slot := range slots {
			value := pools[s][stream.IntN(len(pools[s]))]
			pools[s] = withoutValue(pools[s], value)
			values[slot] = value
		}
		text, err := req.Template.Render(values)
		if err != nil {
			return nil, err
		}
		koans = append(koans, Koan{
			Seed:     req.Seed,
			Index:    i,
			Template: req.Name,
			Slots:    values,
			Text:     text,
		})
	}
	return koans, nil
}

// distinctCount counts the distinct values in a bag: how many draws it can
// supply, since drawing a value retires every copy of it.
func distinctCount(values []string) int {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	return len(seen)
}

// withoutValue returns pool with every copy of value gone. It writes into
// pool's own array, which Draw owns.
func withoutValue(pool []string, value string) []string {
	return slices.DeleteFunc(pool, func(v string) bool { return v == value })
}

// ExhaustedError reports a bag that could not supply another distinct value.
// The CLI maps it to exit 3 and adds the remedy.
type ExhaustedError struct {
	// Name is the bag, which is also the slot name.
	Name string
	// Path is the bag file.
	Path string
	// Available is how many distinct values the pool held at the start of the
	// run, after any --used exclusion.
	Available int
	// Needed is how many draws were asked for.
	Needed int
}

// Error implements error.
func (e *ExhaustedError) Error() string {
	draws := "draws"
	if e.Needed == 1 {
		draws = "draw"
	}
	return fmt.Sprintf("bag %s (%s) has %d distinct values, not enough for %d %s",
		e.Name, e.Path, e.Available, e.Needed, draws)
}

// UnboundSlotError reports a template slot with no bag in Request.Bags. It is
// a guard: the CLI loads bags for exactly the template's slots, so it meets a
// *MissingBagError first. The CLI maps it to exit 4.
type UnboundSlotError struct {
	// Slot is the slot name, without its braces.
	Slot string
}

// Error implements error.
func (e *UnboundSlotError) Error() string {
	return fmt.Sprintf("no bag bound to slot {%s}", e.Slot)
}
