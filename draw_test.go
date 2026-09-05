package koan

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"testing"
)

// inlineBag builds a bag the way the loader would, without touching the disk:
// draw tests care about values and paths, not about parsing.
func inlineBag(name string, values ...string) *Bag {
	return &Bag{
		Name:   name,
		Path:   filepath.Join("testdata", "bags", name+".txt"),
		Format: FormatTxt,
		Values: values,
	}
}

// bagsOf keys bags by name, the shape Request.Bags wants.
func bagsOf(bags ...*Bag) map[string]*Bag {
	m := make(map[string]*Bag, len(bags))
	for _, bag := range bags {
		m[bag.Name] = bag
	}
	return m
}

func mustDraw(t *testing.T, req Request) []Koan {
	t.Helper()
	koans, err := Draw(req)
	if err != nil {
		t.Fatalf("Draw(%+v) = error %v, want koans", req, err)
	}
	return koans
}

// TestDrawDeterministic pins the promise the seed makes: the same request twice
// is byte-identical, so a logged seed reproduces a run.
func TestDrawDeterministic(t *testing.T) {
	req := Request{
		Template: ParseTemplate("{adjective} {muse}"),
		Name:     "art-director.txt",
		Bags: bagsOf(
			inlineBag("adjective", "chalky", "brittle", "molten", "damp", "loud"),
			inlineBag("muse", "cartographer", "welder", "archivist", "diver", "printer"),
		),
		Seed:  412,
		Count: 4,
	}

	first, second := mustDraw(t, req), mustDraw(t, req)

	a, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("two draws with seed 412 differ:\n %s\n %s", a, b)
	}
}

// TestDrawPrefixStable pins that -n 5 extends -n 3 rather than replacing it:
// each draw consumes exactly one random number per slot.
func TestDrawPrefixStable(t *testing.T) {
	req := func(n int) Request {
		return Request{
			Template: ParseTemplate("{adjective} {muse}"),
			Name:     "art-director.txt",
			Bags: bagsOf(
				inlineBag("adjective", "chalky", "brittle", "molten", "damp", "loud", "thin"),
				inlineBag("muse", "cartographer", "welder", "archivist", "diver", "printer", "smith"),
			),
			Seed:  99,
			Count: n,
		}
	}

	short, long := mustDraw(t, req(3)), mustDraw(t, req(5))
	if len(short) != 3 || len(long) != 5 {
		t.Fatalf("counts = %d and %d, want 3 and 5", len(short), len(long))
	}
	for i, koan := range short {
		if !maps.Equal(koan.Slots, long[i].Slots) || koan.Text != long[i].Text {
			t.Errorf("draw %d differs: -n 3 gave %+v, -n 5 gave %+v", i, koan, long[i])
		}
	}
}

// TestDrawNoRepeatWithinBag pins without-replacement: n draws from a bag of n
// distinct values use each value exactly once.
func TestDrawNoRepeatWithinBag(t *testing.T) {
	values := []string{"chalky", "brittle", "molten", "damp", "loud"}
	koans := mustDraw(t, Request{
		Template: ParseTemplate("{adjective}"),
		Name:     "-",
		Bags:     bagsOf(inlineBag("adjective", values...)),
		Seed:     7,
		Count:    len(values),
	})

	var drawn []string
	for _, koan := range koans {
		drawn = append(drawn, koan.Slots["adjective"])
	}
	slices.Sort(drawn)
	want := slices.Sorted(slices.Values(values))
	if !slices.Equal(drawn, want) {
		t.Errorf("drew %q, want each of %q once", drawn, want)
	}
}

// TestDrawWeightRemovesEveryCopy pins that a duplicated line is a weight, not a
// second chance: drawing it removes every copy for the rest of the run.
func TestDrawWeightRemovesEveryCopy(t *testing.T) {
	for seed := range uint64(20) {
		koans := mustDraw(t, Request{
			Template: ParseTemplate("{weighted}"),
			Name:     "-",
			Bags:     bagsOf(inlineBag("weighted", "a", "a", "b")),
			Seed:     seed,
			Count:    2,
		})
		drawn := []string{koans[0].Text, koans[1].Text}
		slices.Sort(drawn)
		if !slices.Equal(drawn, []string{"a", "b"}) {
			t.Errorf("seed %d drew %q, want one a and one b", seed, drawn)
		}
	}
}

// TestDrawExhausted pins that a pool too small for n fails up front, naming the
// bag and both counts, rather than repeating a value or returning part of a run.
func TestDrawExhausted(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "brittle")
	koans, err := Draw(Request{
		Template: ParseTemplate("{adjective}"),
		Name:     "-",
		Bags:     bagsOf(bag),
		Seed:     1,
		Count:    3,
	})
	if koans != nil {
		t.Errorf("koans = %+v, want none alongside an error", koans)
	}
	e := wantErr[*ExhaustedError](t, err)
	if e.Name != "adjective" {
		t.Errorf("Name = %q, want %q", e.Name, "adjective")
	}
	if e.Path != bag.Path {
		t.Errorf("Path = %q, want %q", e.Path, bag.Path)
	}
	if e.Available != 2 {
		t.Errorf("Available = %d, want 2", e.Available)
	}
	if e.Needed != 3 {
		t.Errorf("Needed = %d, want 3", e.Needed)
	}
	wantContains(t, e.Error(), "adjective", bag.Path, "2", "3")
}

// TestDrawExhaustedCountsDistinct pins that weights do not pad the pool: a bag
// of three lines holding two distinct values can supply only two draws.
func TestDrawExhaustedCountsDistinct(t *testing.T) {
	_, err := Draw(Request{
		Template: ParseTemplate("{weighted}"),
		Name:     "-",
		Bags:     bagsOf(inlineBag("weighted", "a", "a", "b")),
		Seed:     1,
		Count:    3,
	})
	e := wantErr[*ExhaustedError](t, err)
	if e.Available != 2 {
		t.Errorf("Available = %d, want 2 distinct values", e.Available)
	}
}

// TestDrawExhaustedNamesTightestBag pins that exhaustion reports the ceiling
// for the whole run, not the first bag that happens to be short: the remedy
// quotes Available, so naming a roomier bag sends the caller to a count that
// still fails.
func TestDrawExhaustedNamesTightestBag(t *testing.T) {
	bags := bagsOf(
		inlineBag("a", "1", "2", "3", "4", "5"),
		inlineBag("b", "1", "2"),
		inlineBag("c", "1", "2", "3"),
	)
	_, err := Draw(Request{
		Template: ParseTemplate("{a} {b} {c}"),
		Name:     "-",
		Bags:     bags,
		Seed:     1,
		Count:    10,
	})
	e := wantErr[*ExhaustedError](t, err)
	if e.Name != "b" {
		t.Errorf("Name = %q, want %q: b is the tightest bag", e.Name, "b")
	}
	if e.Available != 2 {
		t.Errorf("Available = %d, want 2", e.Available)
	}
	if e.Needed != 10 {
		t.Errorf("Needed = %d, want 10", e.Needed)
	}
}

// TestDrawExhaustedNamesFirstSlot pins how a tie breaks when two bags are
// equally short: the first in template order, so the message is stable.
func TestDrawExhaustedNamesFirstSlot(t *testing.T) {
	bags := bagsOf(inlineBag("a", "one"), inlineBag("b", "two"))
	for _, tc := range []struct{ source, want string }{
		{"{a} {b}", "a"},
		{"{b} {a}", "b"},
	} {
		_, err := Draw(Request{
			Template: ParseTemplate(tc.source),
			Name:     "-",
			Bags:     bags,
			Seed:     1,
			Count:    2,
		})
		e := wantErr[*ExhaustedError](t, err)
		if e.Name != tc.want {
			t.Errorf("template %q reported bag %q, want %q", tc.source, e.Name, tc.want)
		}
	}
}

// TestDrawUnboundSlot pins the guard for a slot with no bag in the request.
func TestDrawUnboundSlot(t *testing.T) {
	koans, err := Draw(Request{
		Template: ParseTemplate("{adjective} {muse}"),
		Name:     "-",
		Bags:     bagsOf(inlineBag("adjective", "chalky", "brittle")),
		Seed:     1,
		Count:    1,
	})
	if koans != nil {
		t.Errorf("koans = %+v, want none alongside an error", koans)
	}
	e := wantErr[*UnboundSlotError](t, err)
	if e.Slot != "muse" {
		t.Errorf("Slot = %q, want %q", e.Slot, "muse")
	}
	wantContains(t, e.Error(), "muse")
}

// TestDrawRepeatedSlot pins that a slot used twice draws once: the value repeats
// in the text, and a bag of two values still supports two draws.
func TestDrawRepeatedSlot(t *testing.T) {
	koans := mustDraw(t, Request{
		Template: ParseTemplate("{adjective} on {adjective}"),
		Name:     "-",
		Bags:     bagsOf(inlineBag("adjective", "chalky", "brittle")),
		Seed:     3,
		Count:    2,
	})
	for i, koan := range koans {
		if len(koan.Slots) != 1 {
			t.Errorf("koan %d slots = %v, want one entry", i, koan.Slots)
		}
		value := koan.Slots["adjective"]
		if want := value + " on " + value; koan.Text != want {
			t.Errorf("koan %d text = %q, want %q", i, koan.Text, want)
		}
	}
}

// TestDrawSlotOrderFollowsTemplate pins that slots consume the stream in order
// of first appearance: reordering them in the template pairs the values
// differently under one seed.
func TestDrawSlotOrderFollowsTemplate(t *testing.T) {
	bags := bagsOf(
		inlineBag("a", "a1", "a2", "a3", "a4", "a5"),
		inlineBag("b", "b1", "b2", "b3", "b4", "b5"),
	)
	draw := func(source string, seed uint64) map[string]string {
		return mustDraw(t, Request{
			Template: ParseTemplate(source),
			Name:     "-",
			Bags:     bags,
			Seed:     seed,
			Count:    1,
		})[0].Slots
	}

	differed := false
	for seed := range uint64(50) {
		if !maps.Equal(draw("{a} {b}", seed), draw("{b} {a}", seed)) {
			differed = true
			break
		}
	}
	if !differed {
		t.Error("reordering the slots never changed the draw; slot order does not reach the stream")
	}
}

// TestDrawKoanFields pins every field of the wire struct.
func TestDrawKoanFields(t *testing.T) {
	koans := mustDraw(t, Request{
		Template: ParseTemplate("Needs more {adjective} {quality}."),
		Name:     "art-director.txt",
		Bags: bagsOf(
			inlineBag("adjective", "chalky", "brittle", "molten"),
			inlineBag("quality", "candor", "restraint", "nerve"),
		),
		Seed:  412,
		Count: 3,
	})

	if len(koans) != 3 {
		t.Fatalf("koans = %d, want 3", len(koans))
	}
	for i, koan := range koans {
		if koan.Seed != 412 {
			t.Errorf("koan %d seed = %d, want 412", i, koan.Seed)
		}
		if koan.Index != i {
			t.Errorf("koan %d index = %d, want %d", i, koan.Index, i)
		}
		if koan.Template != "art-director.txt" {
			t.Errorf("koan %d template = %q, want %q", i, koan.Template, "art-director.txt")
		}
		if len(koan.Slots) != 2 {
			t.Fatalf("koan %d slots = %v, want two entries", i, koan.Slots)
		}
		want := "Needs more " + koan.Slots["adjective"] + " " + koan.Slots["quality"] + "."
		if koan.Text != want {
			t.Errorf("koan %d text = %q, want %q", i, koan.Text, want)
		}
	}

	// Each koan owns its map, so a caller editing one cannot reach another.
	koans[0].Slots["adjective"] = "scribbled"
	if koans[1].Slots["adjective"] == "scribbled" {
		t.Error("koans share one slots map, want a fresh map per koan")
	}
}

// TestKoanJSONKeys pins the JSONL wire shape.
func TestKoanJSONKeys(t *testing.T) {
	koan := Koan{
		Seed:     412,
		Index:    0,
		Template: "art-director.txt",
		Slots:    map[string]string{"adjective": "chalky"},
		Text:     "Needs more chalky.",
	}
	data, err := json.Marshal(koan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := slices.Sorted(maps.Keys(fields))
	want := []string{"i", "seed", "slots", "template", "text"}
	if !slices.Equal(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
}

// TestDrawCountBelowOne pins that a count the CLI should have rejected is not
// an error here: no draws, no complaint.
func TestDrawCountBelowOne(t *testing.T) {
	for _, count := range []int{0, -1} {
		koans, err := Draw(Request{
			Template: ParseTemplate("{adjective}"),
			Name:     "-",
			Bags:     bagsOf(inlineBag("adjective", "chalky")),
			Seed:     1,
			Count:    count,
		})
		if koans != nil || err != nil {
			t.Errorf("Draw with count %d = (%v, %v), want (nil, nil)", count, koans, err)
		}
	}
}

// TestDrawNoSlots pins a template that binds nothing: it still renders n times.
func TestDrawNoSlots(t *testing.T) {
	koans := mustDraw(t, Request{
		Template: ParseTemplate("say it plainly"),
		Name:     "-",
		Bags:     map[string]*Bag{},
		Seed:     1,
		Count:    3,
	})
	if len(koans) != 3 {
		t.Fatalf("koans = %d, want 3", len(koans))
	}
	for i, koan := range koans {
		if koan.Text != "say it plainly" {
			t.Errorf("koan %d text = %q, want %q", i, koan.Text, "say it plainly")
		}
		if koan.Slots == nil {
			t.Errorf("koan %d slots is nil, want an empty map", i)
		}
		if len(koan.Slots) != 0 {
			t.Errorf("koan %d slots = %v, want empty", i, koan.Slots)
		}
	}
}

// TestDrawDoesNotMutateBags pins that the caller's bags survive a run intact:
// the pools Draw shrinks are its own copies.
func TestDrawDoesNotMutateBags(t *testing.T) {
	bag := inlineBag("adjective", "chalky", "chalky", "brittle", "molten")
	before := slices.Clone(bag.Values)

	mustDraw(t, Request{
		Template: ParseTemplate("{adjective}"),
		Name:     "-",
		Bags:     bagsOf(bag),
		Seed:     5,
		Count:    3,
	})

	if !slices.Equal(bag.Values, before) {
		t.Errorf("bag values = %q after the draw, want %q", bag.Values, before)
	}
}
