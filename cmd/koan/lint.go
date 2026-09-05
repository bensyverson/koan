package main

import (
	"fmt"

	"github.com/bensyverson/koan"
	"github.com/spf13/cobra"
)

// lintCommand builds `koan lint`: an assertion verb. It reports everything a
// draw of -n 1 would refuse, for every template at once, without drawing, and
// exits 1 on a finding so `&&` and `||` do the branching.
func (a *app) lintCommand() *cobra.Command {
	var bagsDir string
	var usedDir string
	cmd := &cobra.Command{
		Use:     "lint <template>...",
		Short:   "Report everything a draw would refuse, for every template, without drawing",
		GroupID: groupRead,
		Long: "Check templates without drawing from them: an unreadable template, a slot with\n" +
			"no bag, a bag that will not parse, and — with --used — a bag whose values are all\n" +
			"spent. Each finding is one line, `<template>: <what is wrong>`; a template with\n" +
			"nothing wrong prints `<template>: ok`. Exit 1 when there is any finding, 0 when\n" +
			"there is none.\n\n" +
			"A multi-line template is not a finding: it is valid, and draws under --json.",
		Example: "  # One template, against the bags beside it.\n" +
			"  koan lint examples/art-director/art-director.txt\n\n" +
			"  # Every template in a directory, against one bags directory.\n" +
			"  koan lint patterns/*.txt --bags vocabulary\n\n" +
			"  # Would tonight's round still have values left?\n" +
			"  koan lint art-director.txt --used .koan-used && koan art-director.txt --used .koan-used",
		Args: atLeastOneTemplate,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.lint(args, bagsDir, usedDir)
		},
	}
	cmd.Flags().StringVar(&bagsDir, "bags", "",
		"directory holding the bags for every template (default: each template's own directory)")
	cmd.Flags().StringVar(&usedDir, "used", "",
		"directory of already-drawn values to subtract before checking")
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
func (a *app) lint(paths []string, bagsDir, usedDir string) error {
	a.ctx = remedyContext{BagsDir: bagsDir, UsedDir: usedDir, Count: 1}
	found := false
	for _, path := range paths {
		findings := a.findingsFor(path, bagsDir, usedDir)
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

// findingsFor is everything a draw of one koan from path would refuse. The
// template's own path is the line's prefix, so a finding never restates it.
func (a *app) findingsFor(path, bagsFlag, usedDir string) []string {
	tmpl, err := readTemplate(path, a.stdin)
	if err != nil {
		return []string{"cannot read the template: " + detail(err)}
	}

	var findings []string
	bags, err := koan.LoadBags(bagsDirFor(bagsFlag, path), tmpl.Slots())
	if err != nil {
		findings = append(findings, messages(err)...)
	}
	if usedDir != "" {
		used, err := koan.LoadUsed(usedDir, tmpl.Slots())
		if err != nil {
			findings = append(findings, messages(err)...)
		}
		bags = koan.Exclude(bags, used)
	}

	// What is left is the exhaustion a draw would report, in slot order.
	for _, slot := range tmpl.Slots() {
		bag, ok := bags[slot]
		if !ok {
			continue
		}
		if available := len(distinct(bag.Values)); available < 1 {
			spent := &koan.ExhaustedError{
				Name:      bag.Name,
				Path:      bag.Path,
				Available: available,
				Needed:    1,
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
