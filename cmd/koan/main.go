// Command koan is the thin Cobra adapter over the koan library. It holds no
// decision the library does not.
package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "koan",
		Short: "Draw seeded random values from hand-curated bags into a template",
	}
	if err := root.Execute(); err != nil {
		os.Exit(2)
	}
}
