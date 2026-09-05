package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bensyverson/koan"
	"github.com/spf13/cobra"
)

// clockSeedModulus keeps a clock-derived seed to six digits, so it retypes
// out of a log without transcription errors.
const clockSeedModulus = 1_000_000

// clockSeed is the seed for a run that named none: the microseconds of the
// current second. Microseconds rather than nanoseconds because macOS reports
// wall-clock time at microsecond resolution, so nanoseconds modulo a million
// always ended in 000 and only a thousand seeds were ever possible.
func clockSeed() uint64 {
	return uint64(time.Now().UnixMicro()) % clockSeedModulus
}

// stdinName is the template argument that means standard input, and the name
// such a draw reports in its JSON.
const stdinName = "-"

// app is one invocation: the streams it talks on, the context its remedies
// need, and the exit code a verb that succeeded still wants to return.
type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	ctx    remedyContext
	exit   exitCode
}

// run parses args, executes the verb they name, and returns the process exit
// code. It is the whole of main.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	a := &app{stdin: stdin, stdout: stdout, stderr: stderr}
	root := a.rootCommand()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		return int(report(stderr, err, a.ctx))
	}
	return int(a.exit)
}

// drawOptions are the root command's flags.
type drawOptions struct {
	bagsDir string
	usedDir string
	seed    uint64
	count   int
	jsonl   bool
}

// rootCommand builds `koan`: the primer when it is bare, the draw when it is
// given a template, and the parent of the read and help verbs.
func (a *app) rootCommand() *cobra.Command {
	opt := &drawOptions{}
	root := &cobra.Command{
		Use:           "koan <template>",
		Short:         "Draw seeded random values from hand-curated bags into a template",
		Long:          primer,
		Example:       rootExample,
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          drawArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.draw(cmd, args, opt)
		},
	}
	root.SetVersionTemplate("koan {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	flags := root.Flags()
	flags.StringVar(&opt.bagsDir, "bags", "",
		"directory holding the bags (default: the template's own directory, \".\" for -)")
	flags.StringVar(&opt.usedDir, "used", "",
		"directory of already-drawn values: excluded before the draw, appended to after it")
	flags.Uint64Var(&opt.seed, "seed", 0,
		"seed the draw (default: a six-digit number from the clock); 0 is a valid seed")
	flags.IntVarP(&opt.count, "count", "n", 1, "how many draws to make from the one seeded stream")
	flags.BoolVar(&opt.jsonl, "json", false, "print JSON Lines, one self-contained object per draw")

	root.AddGroup(
		&cobra.Group{ID: groupRead, Title: "Read verbs (they work with nothing set up):"},
		&cobra.Group{ID: groupHelp, Title: "Help verbs:"},
	)
	root.AddCommand(a.bagsCommand(), a.lintCommand(), recipesCommand(), schemaCommand())
	root.SetHelpCommandGroupID(groupHelp)
	silenceUsage(root)
	return root
}

// Command groups, so the primer lists the verbs by role rather than
// alphabetically.
const (
	groupRead = "read"
	groupHelp = "help"
)

// silenceUsage makes every command in the tree print its own errors and route
// a flag error through the adapter's exit-2 path.
func silenceUsage(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return usagef(fmt.Sprintf("run %s --help for the flags it accepts", c.CommandPath()),
			"%s", err.Error())
	})
	for _, child := range cmd.Commands() {
		silenceUsage(child)
	}
}

// drawArgs accepts one template, or none when the primer is wanted.
func drawArgs(_ *cobra.Command, args []string) error {
	if len(args) <= 1 {
		return nil
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = fmt.Sprintf("%q", arg)
	}
	return usagef(
		"quote the path if it holds spaces, or pass one template; "+
			"run koan lint <template>... to check several at once",
		"koan draws from one template, but %d were given: %s",
		len(args), strings.Join(quoted, " "))
}

// draw is the root command's act: read the template, load the bags, subtract
// the used values, draw, print, and record. Nothing is recorded until every
// draw has succeeded and stdout has been written.
func (a *app) draw(cmd *cobra.Command, args []string, opt *drawOptions) error {
	if len(args) == 0 {
		if cmd.Flags().NFlag() == 0 {
			return cmd.Help()
		}
		return usagef("pass a template path, or - to read one from standard input; "+
			"run koan --help for the flags", "no template given")
	}
	path := args[0]
	a.ctx = remedyContext{UsedDir: opt.usedDir, Count: opt.count}

	if opt.count < 1 {
		return usagef("pass -n 1 or more", "-n must be 1 or more, but it is %d", opt.count)
	}

	tmpl, err := readTemplate(path, a.stdin)
	if err != nil {
		return &cliError{
			code:   exitTarget,
			what:   fmt.Sprintf("cannot read the template %s: %s", path, detail(err)),
			remedy: fmt.Sprintf("check the path, then run koan lint %s", path),
		}
	}
	if tmpl.MultiLine() && !opt.jsonl {
		return usagef("pass --json to get one object per draw, or use a single-line template",
			"the template %s spans %d lines, and plain output is one line per draw",
			path, strings.Count(tmpl.Source(), "\n")+1)
	}

	a.ctx.BagsDir = bagsDirFor(opt.bagsDir, path)
	bags, err := koan.LoadBags(a.ctx.BagsDir, tmpl.Slots())
	if err != nil {
		return err
	}
	if opt.usedDir != "" {
		used, err := koan.LoadUsed(opt.usedDir, tmpl.Slots())
		if err != nil {
			// A used directory is read as a bags directory, so it is the one
			// `koan bags` should be pointed at to see what is wrong with it.
			a.ctx.BagsDir = opt.usedDir
			return err
		}
		bags = koan.Exclude(bags, used)
	}

	seed := opt.seed
	if !cmd.Flags().Changed("seed") {
		seed = clockSeed()
	}
	koans, err := koan.Draw(koan.Request{
		Template: tmpl,
		Name:     path,
		Bags:     bags,
		Seed:     seed,
		Count:    opt.count,
	})
	if err != nil {
		return err
	}

	// The seed goes out only once the draw has succeeded, so a failed run
	// leaves nothing but its error on stderr.
	if _, err := fmt.Fprintf(a.stderr, "seed=%d\n", seed); err != nil {
		return writeFailure(err)
	}
	out := bufio.NewWriter(a.stdout)
	write := writePlain
	if opt.jsonl {
		write = writeJSONL
	}
	if err := write(out, koans); err != nil {
		return writeFailure(err)
	}
	if err := out.Flush(); err != nil {
		return writeFailure(err)
	}

	if opt.usedDir != "" {
		return koan.Record(opt.usedDir, koans)
	}
	return nil
}

// writeFailure reports a stdout or stderr that could not be written. It is an
// environment failure: the draw itself was sound.
func writeFailure(err error) error {
	return &cliError{
		code:   exitEnvironment,
		what:   fmt.Sprintf("cannot write the draws: %s", detail(err)),
		remedy: "check that the command koan is piped into is still reading",
	}
}

// bagsDirFor resolves --bags: the flag when it was given, otherwise the
// template's own directory, and the working directory for stdin.
func bagsDirFor(flag, path string) string {
	switch {
	case flag != "":
		return flag
	case path == stdinName:
		return "."
	default:
		return filepath.Dir(path)
	}
}

// readTemplate parses the template at path, or standard input for "-". The
// error is the filesystem's, unwrapped by the caller for its message.
func readTemplate(path string, stdin io.Reader) (*koan.Template, error) {
	if path == stdinName {
		return koan.ReadTemplate(stdin)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return koan.ReadTemplate(file)
}

// version is the build's version, "devel" for a binary built from a working
// tree with nothing stamped.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
