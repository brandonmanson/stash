package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search keys, types, tags, and metadata (values are encrypted and not searched)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			entries, err := st.Search(args[0])
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintf(os.Stderr, "no matches for %q\n", args[0])
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			for _, e := range entries {
				tags := ""
				if len(e.Tags) > 0 {
					tags = "#" + strings.Join(e.Tags, " #")
				}
				fmt.Fprintf(w, "%s\t[%s]\t%s\n", e.Key, e.Type, tags)
			}
			return w.Flush()
		},
	}
}

func rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "rm <key>",
		Aliases:           []string{"delete"},
		Short:             "Delete a resource",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "deleted %s\n", args[0])
			return nil
		},
	}
}

// useCmd is the reserved seam for the read-vs-use distinction (DD-7/DD-8):
// `use` transfers authority, not custody. Not implemented in the MVP.
func useCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "use <key> ...",
		Short:             "Perform an operation with a resource without revealing it (reserved — not yet implemented)",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("`use` is reserved but not implemented in this MVP.\n\n" +
				"`stash get` transfers CUSTODY (you receive the plaintext).\n" +
				"`use` will transfer AUTHORITY: stash performs the operation on your\n" +
				"behalf and the resource is never revealed. It lands with the stashd\n" +
				"daemon and the execution broker (DD-7/DD-8 in the bundle)")
		},
	}
}
