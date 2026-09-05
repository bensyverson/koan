package koan

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// slotPattern matches a well-formed slot: a brace, a name of one or more
// name characters, a closing brace. Nothing else is a slot, so every other
// brace in a template is literal text.
var slotPattern = regexp.MustCompile(`\{[A-Za-z0-9_.-]+\}`)

// partKind distinguishes the two things a template is made of.
type partKind int

const (
	// partLiteral is text copied to the output unchanged.
	partLiteral partKind = iota
	// partSlot is a slot name whose value is substituted.
	partSlot
)

// part is one literal run or one slot.
type part struct {
	kind partKind
	text string
}

// Template is a parsed template: literal runs and slots in source order.
// The zero Template renders as the empty string. Parse one with
// [ParseTemplate] or [ReadTemplate]; a Template is immutable and safe for
// concurrent use.
type Template struct {
	source string
	parts  []part
	slots  []string
}

// ParseTemplate parses text. It cannot fail: anything that is not a
// well-formed slot is literal text.
//
// A slot is {name} where name matches [A-Za-z0-9_.-]+. There is no escape
// syntax, so "{}" and "{a b}" are literal, and "{{a}}" is a literal brace,
// the slot a, and a literal brace. Trailing carriage returns and newlines are
// stripped, so a one-line template file that ends in a newline is single-line
// and renders without it.
func ParseTemplate(text string) *Template {
	source := strings.TrimRight(text, "\r\n")
	t := &Template{source: source}

	seen := make(map[string]bool)
	last := 0
	for _, m := range slotPattern.FindAllStringIndex(source, -1) {
		if m[0] > last {
			t.parts = append(t.parts, part{kind: partLiteral, text: source[last:m[0]]})
		}
		name := source[m[0]+1 : m[1]-1]
		t.parts = append(t.parts, part{kind: partSlot, text: name})
		if !seen[name] {
			seen[name] = true
			t.slots = append(t.slots, name)
		}
		last = m[1]
	}
	if last < len(source) {
		t.parts = append(t.parts, part{kind: partLiteral, text: source[last:]})
	}
	return t
}

// ReadTemplate parses everything r yields. It returns the reader's error
// unwrapped and no template if the read fails.
func ReadTemplate(r io.Reader) (*Template, error) {
	text, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseTemplate(string(text)), nil
}

// Source returns the text as parsed, after trailing newlines were stripped.
func (t *Template) Source() string { return t.source }

// Slots returns slot names in order of first appearance, each once. The
// result is a fresh copy; mutating it does not affect the template.
func (t *Template) Slots() []string {
	return append([]string(nil), t.slots...)
}

// MultiLine reports whether the source still contains a newline. Plain output
// requires a single-line template; enforcing that is the caller's job.
func (t *Template) MultiLine() bool {
	return strings.Contains(t.source, "\n")
}

// Render substitutes values for slots. A repeated slot repeats its value, and
// values with no matching slot are ignored. A slot with no value in values is
// a *[MissingValueError]: a template never renders short or with a hole.
func (t *Template) Render(values map[string]string) (string, error) {
	var b strings.Builder
	b.Grow(len(t.source))
	for _, p := range t.parts {
		if p.kind == partLiteral {
			b.WriteString(p.text)
			continue
		}
		value, ok := values[p.text]
		if !ok {
			return "", &MissingValueError{Slot: p.text}
		}
		b.WriteString(value)
	}
	return b.String(), nil
}

// MissingValueError reports a slot Render was given no value for.
type MissingValueError struct {
	// Slot is the slot name, without its braces.
	Slot string
}

// Error describes the slot that had no value.
func (e *MissingValueError) Error() string {
	return fmt.Sprintf("no value for slot {%s}", e.Slot)
}
