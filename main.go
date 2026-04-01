// ditty converts line-interactive programs (REPLs, debuggers, etc.) into
// command-line programs.
package main

import (
	"fmt"
	"os"

	"github.com/viettrungluu/ditty/internal/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
