package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brandonmanson/stash/internal/resource"
)

func reserveCmd() *cobra.Command {
	var typ, like, description string
	var tags []string
	cmd := &cobra.Command{
		Use:   "reserve <key>",
		Short: "Claim a key (and type) before you have its value",
		Long: `Reserve a key: the leaf and its type are declared and protected by the
namespace collision rules, but no value exists yet. Reserved keys show up in
ls as (reserved) — a checklist of what's still to collect — and get refuses
them with a fill hint. Fill one with a normal stash put.

--like stamps a whole shape: every leaf under the source subtree is reserved
under the new prefix with the same type and tags. An existing engagement IS
the template:

  stash reserve agency.engagements.acme --like agency.engagements.oldclient

Reserving needs no unlock — there is nothing to encrypt.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSuffix(args[0], ".")
			if err := resource.ValidateKey(key); err != nil {
				return err
			}
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if like != "" {
				like = strings.TrimSuffix(like, ".")
				src, err := st.List(like)
				if err != nil {
					return err
				}
				if len(src) == 0 {
					return fmt.Errorf("nothing under %q to reserve --like", like)
				}
				failed := 0
				for _, e := range src {
					dest := key + strings.TrimPrefix(e.Key, like)
					dissolved, err := st.Reserve(resource.Resource{Key: dest, Type: e.Type, Tags: e.Tags})
					if err != nil {
						fmt.Fprintf(os.Stderr, "skip %s: %v\n", dest, err)
						failed++
						continue
					}
					printDissolved(dissolved)
					fmt.Printf("reserved %s (%s)\n", dest, e.Type)
				}
				if failed > 0 {
					return fmt.Errorf("%d of %d reservations skipped", failed, len(src))
				}
				return nil
			}

			if typ == "" {
				typ = resource.InferType(key)
			}
			dissolved, err := st.Reserve(resource.Resource{Key: key, Type: typ, Tags: tags, Description: description})
			if err != nil {
				return err
			}
			printDissolved(dissolved)
			fmt.Printf("reserved %s (%s) — fill it with `stash put %s`\n", key, typ, key) // key NAMES are non-secret by design — @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-sensitive-data-in-logs:accepted-risk:2026-10-23
			return nil
		},
	}
	cmd.Flags().StringVarP(&typ, "type", "t", "", "resource type (default inferred from the key)")
	cmd.Flags().StringVar(&like, "like", "", "reserve the shape of an existing subtree under this key")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "free-text description (drives recall)")
	cmd.RegisterFlagCompletionFunc("like", keyCompletion)
	return cmd
}

func printDissolved(keys []string) {
	for _, d := range keys {
		fmt.Fprintf(os.Stderr, "note: reservation %s dissolved into a namespace\n", d)
	}
}
