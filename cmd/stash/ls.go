package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brandonmanson/stash/internal/resource"
)

func lsCmd() *cobra.Command {
	var unfilled bool
	cmd := &cobra.Command{
		Use:               "ls [prefix]",
		Short:             "List resources as a namespace tree",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix := ""
			if len(args) == 1 {
				prefix = strings.TrimSuffix(args[0], ".")
			}
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			entries, err := st.List(prefix)
			if err != nil {
				return err
			}
			if unfilled {
				var kept []resource.Entry
				for _, e := range entries {
					if e.Reserved {
						kept = append(kept, e)
					}
				}
				entries = kept
			}
			if len(entries) == 0 {
				if prefix == "" {
					fmt.Println("(empty stash — try `stash put`)")
				} else {
					fmt.Printf("(nothing under %s)\n", prefix)
				}
				return nil
			}
			printTree(entries, prefix)
			return nil
		},
	}
	cmd.Flags().BoolVar(&unfilled, "unfilled", false, "show only reserved (not yet filled) resources")
	return cmd
}

// printTree renders entries as an indented namespace tree rooted at prefix.
func printTree(entries []resource.Entry, prefix string) {
	if prefix != "" {
		fmt.Println(prefix)
	}
	printed := map[string]bool{}
	for _, e := range entries {
		rel := e.Key
		if prefix != "" {
			if e.Key == prefix {
				rel = ""
			} else {
				rel = strings.TrimPrefix(e.Key, prefix+".")
			}
		}
		segs := strings.Split(rel, ".")
		depth := 0
		if prefix != "" {
			depth = 1
		}
		if rel == "" { // the prefix itself is a leaf
			fmt.Printf("%s%s\n", indent(depth), leafLabel(segsLast(e.Key), e)) // key NAMES are non-secret by design — @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-sensitive-data-in-logs:accepted-risk:2026-10-23
			continue
		}
		for i := 0; i < len(segs)-1; i++ {
			ns := strings.Join(segs[:i+1], ".")
			if !printed[ns] {
				printed[ns] = true
				fmt.Printf("%s%s.\n", indent(depth+i), segs[i])
			}
		}
		fmt.Printf("%s%s\n", indent(depth+len(segs)-1), leafLabel(segs[len(segs)-1], e))
	}
}

func leafLabel(name string, e resource.Entry) string {
	label := fmt.Sprintf("%s  [%s]", name, e.Type)
	if len(e.Tags) > 0 {
		label += "  #" + strings.Join(e.Tags, " #")
	}
	if e.Reserved {
		label += "  (reserved)"
	}
	return label
}

func indent(n int) string { return strings.Repeat("  ", n) }

func segsLast(key string) string {
	segs := strings.Split(key, ".")
	return segs[len(segs)-1]
}
