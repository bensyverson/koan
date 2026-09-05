package main

import (
	"encoding/json"
	"io"

	"github.com/bensyverson/koan"
)

// writePlain prints one rendered line per draw. It is the default shape
// because one draw per line is what head, grep, while read and xargs expect.
func writePlain(w io.Writer, koans []koan.Koan) error {
	for _, drawn := range koans {
		if _, err := io.WriteString(w, drawn.Text+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// writeJSONL prints one JSON object per draw, in order: each line is a whole
// koan.Koan, so a line cut out of a log reproduces without its neighbours.
// HTML escaping is off, since the values are prose rather than markup and an
// escaped apostrophe reads worse for no gain in validity.
func writeJSONL(w io.Writer, koans []koan.Koan) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	for _, drawn := range koans {
		if err := encoder.Encode(drawn); err != nil {
			return err
		}
	}
	return nil
}
