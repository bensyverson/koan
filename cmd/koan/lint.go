package main

import (
	"fmt"

	"github.com/bensyverson/koan"
	"github.com/spf13/cobra"
)

// slotlessFinding is what lint says about a template holding no {slot}. It is
// almost always a bag file handed to koan as a template, and it is never what
// a caller meant: a draw of it renders the same line every time.
const slotlessFinding = "has no {slot}: every draw would render the same line; " +
	"is this a bag rather than a template?"

// lintCommand builds `koan lint`: an assertion verb. It reports everything a
// draw of -n would refuse, for every template at once, without drawing, and
// exits 1 on a finding so `&&` and `||` do the branching.
func (a *app) lintCommand() *cobra.Command {
	var bagsDir string
	var usedDir string
	var count int
	cmd := &cobra.Command{
		Use: "lint <template>...",
		Short: "Report what a draw would refuse, for every template, without drawing " +
			"(a multi-line template is valid; it needs --json)",
		GroupID: groupRead,
		Long: "Check templates without drawing from them: an unreadable template, a template\n" +
			"with no slot, a slot with no bag, a bag that will not parse, and a bag that\n" +
			"cannot supply the round -n asks for. Each finding is one line,\n" +
			"`<template>: <what is wrong>`; a template with nothing wrong prints\n" +
			"`<template>: ok`. Exit 1 when there is any finding, 0 when there is none.\n\n" +
			"Lint the round you are about to draw: -n defaults to 1, so a bag with one\n" +
			"value left passes a bare lint and then fails `koan <template> -n 5` with exit\n" +
			"3. Pass lint the same -n and --used the draw will use.\n\n" +
			"A multi-line template is not a finding: it is valid, and draws under --json.",
		Example: "  # One template, against the bags beside it.\n" +
			"  koan lint examples/art-director/art-director.txt\n\n" +
			"  # Every template in a directory, against one bags directory.\n" +
			"  koan lint patterns/*.txt --bags vocabulary\n\n" +
			"  # Would tonight's round of three still have values left?\n" +
			"  koan lint art-director.txt --used .koan-used -n 3 && " +
			"koan art-director.txt --used .koan-used -n 3",
		Args: atLeastOneTemplate,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.lint(args, bagsDir, usedDir, count)
		},
	}
	cmd.Flags().StringVar(&bagsDir, "bags", "",
		"directory holding the bags for every template (default: each template's own directory)")
	cmd.Flags().StringVar(&usedDir, "used", "",
		"directory of already-drawn values to subtract before checking")
	cmd.Flags().IntVarP(&count, "count", "n", 1,
		"the round to check for: a bag left with fewer distinct values is a finding")
	return cmd
}

// atLeastOneTemplate refuses an invocation with nothing to check.
func atLeastOneTemplate(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return nil
	}
	return usagef("run koan lint <template>...; koan lint dir/*.txt checks a whole directory",
		"koan lint needs at least one template")
}

// lint prints one line per template per finding and sets the exit code.
func (a *app) lint(paths []string, bagsDir, usedDir string, count int) error {
	a.ctx = remedyContext{BagsDir: bagsDir, UsedDir: usedDir, Count: count}
	if count < 1 {
		return usagef("pass -n 1 or more", "-n must be 1 or more, but it is %d", count)
	}
	found := false
	for _, path := range paths {
		findings := a.findingsFor(path, bagsDir, usedDir, count)
		if len(findings) == 0 {
			fmt.Fprintf(a.stdout, "%s: ok\n", path)
			continue
		}
		found = true
		for _, finding := range findings {
			fmt.Fprintf(a.stdout, "%s: %s\n", path, finding)
		}
	}
	if found {
		a.exit = exitFinding
	}
	return nil
}

// findingsFor is everything a draw of count koans from path would refuse. The
// template's own path is the line's prefix, so a finding never restates it.
func (a *app) findingsFor(path, bagsFlag, usedDir string, count int) []string {
	tmpl, err := readTemplate(path, a.stdin)
	if err != nil {
		return []string{"cannot read the template: " + detail(err)}
	}

	var findings []string
	slots := tmpl.Slots()
	if len(slots) == 0 {
		findings = append(findings, slotlessFinding)
	}
	bags, err := koan.LoadBags(bagsDirFor(bagsFlag, path), slots)
	if err != nil {
		findings = append(findings, messages(err)...)
	}
	if usedDir != "" {
		used, err := koan.LoadUsed(usedDir, slots)
		if err != nil {
			findings = append(findings, messages(err)...)
		}
		bags = koan.Exclude(bags, used)
	}

	// What is left is the exhaustion a draw would report, in slot order.
	for _, slot := range slots {
		bag, ok := bags[slot]
		if !ok {
			continue
		}
		if available := len(distinct(bag.Values)); available < count {
			spent := &koan.ExhaustedError{
				Name:      bag.Name,
				Path:      bag.Path,
				Available: available,
				Needed:    count,
			}
			findings = append(findings, spent.Error())
		}
	}
	return findings
}

// messages is one string per leaf of a joined error, so several broken bags
// report one finding each.
func messages(err error) []string {
	problems := flatten(err)
	out := make([]string, 0, len(problems))
	for _, problem := range problems {
		out = append(out, problem.Error())
	}
	return out
}
