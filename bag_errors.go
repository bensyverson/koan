package koan

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
)

// Every bag failure is a distinct type carrying the path and, where there is
// one, the 1-based line, so a caller can map each to its own exit code and
// remedy with errors.As. Each message names the file, says what happened in
// plain words, and where the fix is in the file, says what to do about it.

// MissingBagError reports that a bags directory holds neither the .txt nor the
// .csv file for a name. It carries both paths that were tried.
type MissingBagError struct {
	// Name is the bag name that could not be resolved.
	Name string
	// Dir is the bags directory that was searched.
	Dir string
	// TxtPath and CSVPath are the two paths that were tried.
	TxtPath string
	CSVPath string
}

// Error implements error.
func (e *MissingBagError) Error() string {
	return fmt.Sprintf("no bag named %q in %s: tried %s and %s; create one of them",
		e.Name, e.Dir, e.TxtPath, e.CSVPath)
}

// AmbiguousBagError reports that both the .txt and the .csv file for a name
// exist. koan never picks one: a precedence would silently ignore a file its
// author wrote on purpose.
type AmbiguousBagError struct {
	// Name is the bag name backed by two files.
	Name string
	// TxtPath and CSVPath are the two files that exist.
	TxtPath string
	CSVPath string
}

// Error implements error.
func (e *AmbiguousBagError) Error() string {
	return fmt.Sprintf("bag %q is ambiguous: both %s and %s exist; remove one of them",
		e.Name, e.TxtPath, e.CSVPath)
}

// EmptyBagError reports a bag file that yielded no values: every line was
// blank or a comment.
type EmptyBagError struct {
	// Path is the file that holds no values.
	Path string
}

// Error implements error.
func (e *EmptyBagError) Error() string {
	return fmt.Sprintf("bag %s has no values: every line is blank or a %s comment; add a value",
		e.Path, commentPrefix)
}

// EmptyValueError reports a .csv row whose first column is empty. A bag can
// never hold a hole, so this is a hard error rather than a skipped row.
type EmptyValueError struct {
	// Path is the .csv file.
	Path string
	// Line is the 1-based line the row starts on.
	Line int
}

// Error implements error.
func (e *EmptyValueError) Error() string {
	return fmt.Sprintf("%s line %d: the first column is empty; quote the value or delete the row",
		e.Path, e.Line)
}

// CSVError reports a row that is not valid RFC 4180.
type CSVError struct {
	// Path is the .csv file.
	Path string
	// Line is the 1-based line the parser stopped on, 0 if it is not known.
	Line int
	// Err is the underlying encoding/csv error.
	Err error
}

// Error implements error.
func (e *CSVError) Error() string {
	// A *csv.ParseError restates the line, which this message already names.
	detail := e.Err.Error()
	var parse *csv.ParseError
	if errors.As(e.Err, &parse) && parse.Err != nil {
		detail = parse.Err.Error()
	}
	return fmt.Sprintf("%s line %d: cannot parse the row: %s; a value holding a comma or a quote "+
		"must be wrapped in double quotes, with each inner quote doubled", e.Path, e.Line, detail)
}

// Unwrap returns the underlying encoding/csv error.
func (e *CSVError) Unwrap() error { return e.Err }

// ReadError reports a bag file or bags directory that could not be read.
type ReadError struct {
	// Path is the file or directory.
	Path string
	// Err is the underlying filesystem error.
	Err error
}

// Error implements error.
func (e *ReadError) Error() string {
	// An *fs.PathError restates the path, which this message already names.
	detail := e.Err.Error()
	var pathErr *fs.PathError
	if errors.As(e.Err, &pathErr) && pathErr.Err != nil {
		detail = pathErr.Err.Error()
	}
	return fmt.Sprintf("cannot read %s: %s", e.Path, detail)
}

// Unwrap returns the underlying filesystem error.
func (e *ReadError) Unwrap() error { return e.Err }
