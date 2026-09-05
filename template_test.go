package koan

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestParseTemplateSlotsAreInOrderOfFirstAppearance(t *testing.T) {
	tpl := ParseTemplate("{muse} and {adjective}, then {muse} again, then {quality}")
	want := []string{"muse", "adjective", "quality"}
	if got := tpl.Slots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Slots() = %#v, want %#v", got, want)
	}
}

func TestRenderRepeatsARepeatedSlot(t *testing.T) {
	tpl := ParseTemplate("{muse} meets {muse}")
	got, err := tpl.Render(map[string]string{"muse": "cartographer"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := "cartographer meets cartographer"; got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestParseTemplateTreatsMalformedBracesAsLiteralText(t *testing.T) {
	cases := []struct {
		name   string
		source string
		slots  []string
		values map[string]string
		want   string
	}{
		{
			name:   "empty braces",
			source: "{}",
			slots:  nil,
			values: nil,
			want:   "{}",
		},
		{
			name:   "space in name",
			source: "{a b}",
			slots:  nil,
			values: nil,
			want:   "{a b}",
		},
		{
			name:   "doubled braces are a literal brace, a slot and a literal brace",
			source: "{{a}}",
			slots:  []string{"a"},
			values: map[string]string{"a": "X"},
			want:   "{X}",
		},
		{
			name:   "adjacent slots",
			source: "{a}{b}",
			slots:  []string{"a", "b"},
			values: map[string]string{"a": "X", "b": "Y"},
			want:   "XY",
		},
		{
			name:   "unclosed brace",
			source: "a { b",
			slots:  nil,
			values: nil,
			want:   "a { b",
		},
		{
			name:   "stray closing brace",
			source: "a } b",
			slots:  nil,
			values: nil,
			want:   "a } b",
		},
		{
			name:   "unterminated slot name",
			source: "{muse",
			slots:  nil,
			values: nil,
			want:   "{muse",
		},
		{
			name:   "brace inside a slot name",
			source: "{a{b}",
			slots:  []string{"b"},
			values: map[string]string{"b": "Y"},
			want:   "{aY",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl := ParseTemplate(c.source)
			if got := tpl.Slots(); !equalSlots(got, c.slots) {
				t.Errorf("Slots() = %#v, want %#v", got, c.slots)
			}
			got, err := tpl.Render(c.values)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if got != c.want {
				t.Errorf("Render() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseTemplateAcceptsDotsDashesUnderscoresAndDigitsInSlotNames(t *testing.T) {
	tpl := ParseTemplate("{a.b} {c-d} {e_f} {g1} {H.i-j_2}")
	want := []string{"a.b", "c-d", "e_f", "g1", "H.i-j_2"}
	if got := tpl.Slots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Slots() = %#v, want %#v", got, want)
	}
	got, err := tpl.Render(map[string]string{
		"a.b": "1", "c-d": "2", "e_f": "3", "g1": "4", "H.i-j_2": "5",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := "1 2 3 4 5"; got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestParseTemplateWithOnlyLiteralText(t *testing.T) {
	const source = "no slots here at all"
	tpl := ParseTemplate(source)
	if got := tpl.Slots(); len(got) != 0 {
		t.Errorf("Slots() = %#v, want none", got)
	}
	if got := tpl.Source(); got != source {
		t.Errorf("Source() = %q, want %q", got, source)
	}
	got, err := tpl.Render(nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got != source {
		t.Errorf("Render() = %q, want %q", got, source)
	}
}

func TestParseTemplateEmpty(t *testing.T) {
	tpl := ParseTemplate("")
	if got := tpl.Slots(); len(got) != 0 {
		t.Errorf("Slots() = %#v, want none", got)
	}
	if got := tpl.Source(); got != "" {
		t.Errorf("Source() = %q, want empty", got)
	}
	if tpl.MultiLine() {
		t.Error("MultiLine() = true, want false")
	}
	got, err := tpl.Render(nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}

func TestParseTemplateStripsTrailingNewlines(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"unix", "Needs more {adjective} candor.\n"},
		{"windows", "Needs more {adjective} candor.\r\n"},
		{"several", "Needs more {adjective} candor.\n\n\n"},
		{"mixed", "Needs more {adjective} candor.\r\n\r\n"},
		{"none", "Needs more {adjective} candor."},
	}
	const want = "Needs more {adjective} candor."
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tpl := ParseTemplate(c.source)
			if got := tpl.Source(); got != want {
				t.Errorf("Source() = %q, want %q", got, want)
			}
			if tpl.MultiLine() {
				t.Error("MultiLine() = true, want false")
			}
			got, err := tpl.Render(map[string]string{"adjective": "chalky"})
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if want := "Needs more chalky candor."; got != want {
				t.Errorf("Render() = %q, want %q", got, want)
			}
		})
	}
}

func TestParseTemplateReportsAnInteriorNewlineAsMultiLine(t *testing.T) {
	tpl := ParseTemplate("Brief: {muse}\nConstraint: {constraint}\n")
	if !tpl.MultiLine() {
		t.Error("MultiLine() = false, want true")
	}
	if want := "Brief: {muse}\nConstraint: {constraint}"; tpl.Source() != want {
		t.Errorf("Source() = %q, want %q", tpl.Source(), want)
	}
	got, err := tpl.Render(map[string]string{"muse": "cartographer", "constraint": "one typeface"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := "Brief: cartographer\nConstraint: one typeface"; got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestParseTemplateReportsALoneNewlineAsSingleLine(t *testing.T) {
	tpl := ParseTemplate("\n")
	if tpl.MultiLine() {
		t.Error("MultiLine() = true, want false")
	}
	if got := tpl.Source(); got != "" {
		t.Errorf("Source() = %q, want empty", got)
	}
}

func TestReadTemplate(t *testing.T) {
	tpl, err := ReadTemplate(strings.NewReader("Channel your inner {muse}.\n"))
	if err != nil {
		t.Fatalf("ReadTemplate() error = %v", err)
	}
	if want := "Channel your inner {muse}."; tpl.Source() != want {
		t.Errorf("Source() = %q, want %q", tpl.Source(), want)
	}
	if got, want := tpl.Slots(), []string{"muse"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Slots() = %#v, want %#v", got, want)
	}
	if tpl.MultiLine() {
		t.Error("MultiLine() = true, want false")
	}
}

func TestReadTemplateReturnsTheReaderError(t *testing.T) {
	want := errors.New("disk on fire")
	tpl, err := ReadTemplate(errReader{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("ReadTemplate() error = %v, want %v", err, want)
	}
	if tpl != nil {
		t.Errorf("ReadTemplate() = %#v, want nil template on error", tpl)
	}
}

func TestRenderWithoutAValueForASlotFails(t *testing.T) {
	tpl := ParseTemplate("Needs more {adjective} {quality}.")
	got, err := tpl.Render(map[string]string{"adjective": "chalky"})
	if got != "" {
		t.Errorf("Render() = %q, want empty on error", got)
	}
	var missing *MissingValueError
	if !errors.As(err, &missing) {
		t.Fatalf("Render() error = %v (%T), want *MissingValueError", err, err)
	}
	if missing.Slot != "quality" {
		t.Errorf("Slot = %q, want %q", missing.Slot, "quality")
	}
	if !strings.Contains(missing.Error(), "quality") {
		t.Errorf("Error() = %q, want it to name the slot", missing.Error())
	}
}

func TestRenderIgnoresValuesWithNoSlot(t *testing.T) {
	tpl := ParseTemplate("Channel your inner {muse}.")
	got, err := tpl.Render(map[string]string{"muse": "cartographer", "unused": "ignored"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := "Channel your inner cartographer."; got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestRenderAcceptsAnEmptyValue(t *testing.T) {
	tpl := ParseTemplate("[{muse}]")
	got, err := tpl.Render(map[string]string{"muse": ""})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if want := "[]"; got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestSlotsReturnsACopy(t *testing.T) {
	tpl := ParseTemplate("{muse} {quality}")
	first := tpl.Slots()
	first[0] = "clobbered"
	second := tpl.Slots()
	if want := []string{"muse", "quality"}; !reflect.DeepEqual(second, want) {
		t.Fatalf("Slots() = %#v after mutating an earlier result, want %#v", second, want)
	}
}

// equalSlots compares slot lists, treating nil and empty as the same.
func equalSlots(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

// errReader fails every Read, so ReadTemplate's error path can be exercised.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

var _ io.Reader = errReader{}
