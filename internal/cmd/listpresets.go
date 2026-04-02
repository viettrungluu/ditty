package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/viettrungluu/ditty/internal/preset"
)

// newListPresetsCmd creates the `ditty list-presets` subcommand.
func newListPresetsCmd() *cobra.Command {
	var noBuiltinPresets bool
	var presetsFile string

	cmd := &cobra.Command{
		Use:   "list-presets",
		Short: "List available presets",
		Long:  `Lists all available presets (user and built-in) with their names, match patterns, and flags.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve default presets file if not specified.
			if presetsFile == "" {
				if p, err := preset.DefaultPresetsFile(); err == nil {
					presetsFile = p
				}
			}

			var entries []preset.Entry

			// Load user presets.
			if presetsFile != "" {
				userEntries, err := preset.LoadFile(presetsFile)
				if err != nil {
					if !os.IsNotExist(err) {
						return fmt.Errorf("load presets file: %w", err)
					}
				}
				for _, e := range userEntries {
					entries = append(entries, e)
				}
			}

			// Add built-ins.
			if !noBuiltinPresets {
				entries = append(entries, preset.Builtins()...)
			}

			if len(entries) == 0 {
				fmt.Println("No presets available.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tPATTERN\tFLAGS\n")
			for _, e := range entries {
				pattern := "(none)"
				if len(e.CommandRegexes) > 0 {
					pattern = e.CommandRegexes[0].String()
					for i := 1; i < len(e.CommandRegexes); i++ {
						pattern += ", " +
							e.CommandRegexes[i].String()
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n",
					e.Name, pattern, e.Flags)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().BoolVar(&noBuiltinPresets, "no-builtin-presets", false,
		"only show user presets")
	cmd.Flags().StringVar(&presetsFile, "presets-file", "",
		"path to presets file (default: ~/.ditty/presets)")

	return cmd
}
