package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bensyverson/koan"
)

// remedyPrefix opens the second line of every failure: the next command to run
// or the edit to make. It is a fixed prefix so a caller can grep for it.
const remedyPrefix = "remedy: "

// messagePrefix opens every line that says what went wrong.
const messagePrefix = "koan: "

// exitCode is a process exit status from the house table shared by every tool
// in the house; see project/agents/cli-design.md and the plan's exit table.
type exitCode int

const (
	// exitOK means the draw happened, lint found nothing, or a help verb ran.
	exitOK exitCode = 0
	// exitFinding means lint ran and found something.
	exitFinding exitCode = 1
	// exitUsage means the invocation itself was malformed.
	exitUsage exitCode = 2
	// exitExhausted means a bag could not supply another distinct value.
	exitExhausted exitCode = 3
	// exitTarget means a template, bag or used directory could not be loaded.
	exitTarget exitCode = 4
	// exitEnvironment means the used directory could not be created or written.
	exitEnvironment exitCode = 5
)

// cliError is a failure the adapter itself reports, carrying its own exit
// code, message and remedy. Library errors are classified instead, so that
// the adapter never restates what the library already says.
type cliError struct {
	code   exitCode
	what   string
	remedy string
}

// Error implements error.
func (e *cliError) Error() string { return e.what }

// usagef builds an exit-2 failure: a malformed invocation, refused before any
// work is done.
func usagef(remedy, format string, a ...any) *cliError {
	return &cliError{code: exitUsage, what: fmt.Sprintf(format, a...), remedy: remedy}
}

// remedyContext is what a remedy needs beyond the error itself: the
// directories the run was working with, and how many draws it asked for.
type remedyContext struct {
	// BagsDir is the bags directory in play, empty before one is chosen.
	BagsDir string
	// UsedDir is the --used directory, empty when the flag was not given.
	UsedDir string
	// Count is the -n the run asked for.
	Count int
}

// report writes err to w as a message line per problem followed by one remedy
// line, and returns the exit code the failure maps to. A nil error reports
// nothing and exits 0.
func report(w io.Writer, err error, ctx remedyContext) exitCode {
	if err == nil {
		return exitOK
	}
	if cli, ok := errors.AsType[*cliError](err); ok {
		fmt.Fprintf(w, "%s%s\n", messagePrefix, cli.what)
		fmt.Fprintf(w, "%s%s\n", remedyPrefix, cli.remedy)
		return cli.code
	}

	problems := flatten(err)
	code := exitOK
	for _, problem := range problems {
		fmt.Fprintf(w, "%s%s\n", messagePrefix, problem.Error())
		code = max(code, classify(problem))
	}
	fmt.Fprintf(w, "%s%s\n", remedyPrefix, remedyFor(problems, ctx))
	return code
}

// flatten expands the tree errors.Join builds into the leaves that carry the
// messages, so several broken bags report one line each.
func flatten(err error) []error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}
	var leaves []error
	for _, child := range joined.Unwrap() {
		leaves = append(leaves, flatten(child)...)
	}
	return leaves
}

// classify maps one library error to its exit code. Anything unrecognised is
// a target failure: the adapter would rather say "could not load that" than
// claim a success it cannot vouch for.
func classify(err error) exitCode {
	switch {
	case is[*koan.ExhaustedError](err):
		return exitExhausted
	case is[*koan.RecordError](err):
		return exitEnvironment
	default:
		return exitTarget
	}
}

// is reports whether err is, or wraps, a T.
func is[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

// remedyFor is the next command or edit for a run that failed with problems.
// Several problems share one remedy, because the library's own messages
// already say what each file needs.
func remedyFor(problems []error, ctx remedyContext) string {
	if len(problems) != 1 {
		return fmt.Sprintf("fix the files named above, then run %s to check them all",
			bagsCommand(ctx.BagsDir))
	}
	err := problems[0]

	if missing, ok := errors.AsType[*koan.MissingBagError](err); ok {
		return fmt.Sprintf("create %s or %s, then run %s to check",
			missing.TxtPath, missing.CSVPath, bagsCommand(missing.Dir))
	}
	if ambiguous, ok := errors.AsType[*koan.AmbiguousBagError](err); ok {
		return fmt.Sprintf("remove one of %s and %s; koan never picks between them",
			ambiguous.TxtPath, ambiguous.CSVPath)
	}
	if empty, ok := errors.AsType[*koan.EmptyBagError](err); ok {
		return fileRemedy("add a value to", empty.Path, ctx)
	}
	if cell, ok := errors.AsType[*koan.EmptyValueError](err); ok {
		return fileRemedy(fmt.Sprintf("fill in or delete line %d of", cell.Line), cell.Path, ctx)
	}
	if bad, ok := errors.AsType[*koan.CSVError](err); ok {
		return fileRemedy(fmt.Sprintf("quote the value on line %d of", bad.Line), bad.Path, ctx)
	}
	if unbound, ok := errors.AsType[*koan.UnboundSlotError](err); ok {
		return fmt.Sprintf("add a bag for {%s} in %s, then run %s to check",
			unbound.Slot, ctx.BagsDir, bagsCommand(ctx.BagsDir))
	}
	if hole, ok := errors.AsType[*koan.MissingValueError](err); ok {
		return fmt.Sprintf("add a bag for {%s}, then run %s to check", hole.Slot, bagsCommand(ctx.BagsDir))
	}
	if spent, ok := errors.AsType[*koan.ExhaustedError](err); ok {
		return exhaustedRemedy(spent, ctx)
	}
	if failed, ok := errors.AsType[*koan.RecordError](err); ok {
		return fmt.Sprintf("check the permissions on %s, or pass another --used directory", failed.Path)
	}
	if unreadable, ok := errors.AsType[*koan.ReadError](err); ok {
		if unreadable.Path == ctx.BagsDir {
			return fmt.Sprintf("create %s, or name a bags directory that exists", unreadable.Path)
		}
		return fileRemedy("check that koan can read", unreadable.Path, ctx)
	}
	return fmt.Sprintf("run %s to check the bags", bagsCommand(ctx.BagsDir))
}

// fileRemedy is the standard two-step for a broken bag file: the edit, then
// the command that confirms it.
func fileRemedy(edit, path string, ctx remedyContext) string {
	dir := ctx.BagsDir
	if dir == "" {
		dir = filepath.Dir(path)
	}
	return fmt.Sprintf("%s %s, then run %s to check", edit, path, bagsCommand(dir))
}

// exhaustedRemedy names the smaller -n that would fit, the bag to grow, and,
// under --used, the used file to prune.
func exhaustedRemedy(spent *koan.ExhaustedError, ctx remedyContext) string {
	var b strings.Builder
	if spent.Available > 0 {
		fmt.Fprintf(&b, "draw -n %d or fewer, or add values to %s", spent.Available, spent.Path)
	} else {
		fmt.Fprintf(&b, "add values to %s", spent.Path)
	}
	if ctx.UsedDir != "" {
		fmt.Fprintf(&b, ", or prune %s", usedFilePath(ctx.UsedDir, spent.Name))
	}
	return b.String()
}

// usedFilePath is the file in the used directory that holds a bag's spent
// values: whichever of the two formats is already there, .txt for a bag with
// no used file yet, which is what Record would create.
func usedFilePath(dir, name string) string {
	csvPath := filepath.Join(dir, name+".csv")
	txtPath := filepath.Join(dir, name+".txt")
	if _, err := os.Stat(txtPath); err == nil {
		return txtPath
	}
	if _, err := os.Stat(csvPath); err == nil {
		return csvPath
	}
	return txtPath
}

// bagsCommand is the `koan bags` invocation that inspects dir.
func bagsCommand(dir string) string {
	if dir == "" || dir == "." {
		return "koan bags"
	}
	return "koan bags " + dir
}

// detail strips the path an *fs.PathError restates, since every message the
// adapter builds already names it.
func detail(err error) string {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	return err.Error()
}
