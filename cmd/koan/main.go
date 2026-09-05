// Command koan is the thin Cobra adapter over the koan library. It holds no
// decision the library does not: it parses the invocation, composes the
// library calls, writes the output, and maps every library error to the house
// exit table in project/agents/cli-design.md.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
