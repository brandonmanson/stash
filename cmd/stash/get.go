package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"stash/internal/resource"
)

func getCmd() *cobra.Command {
	var reveal, metaOnly bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Retrieve a resource's value (secrets are masked on a terminal unless --reveal)",
		Long: `Retrieve a resource's value.

Secret types (credential, password, token, certificate) are masked when stdout
is a terminal; pass --reveal to print them, or pipe the output — piped output
is always the raw value, so $(stash get tokens.foo) just works.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			res, err := st.Get(args[0])
			if err != nil {
				return err
			}

			if metaOnly {
				res.Value = nil
				out, err := json.MarshalIndent(res, "", "  ")
				if err != nil {
					return fmt.Errorf("encoding metadata: %w", err)
				}
				fmt.Println(string(out))
				return nil
			}

			if res.Reserved {
				return fmt.Errorf("%s is reserved but not yet filled — fill it with `stash put %s`", res.Key, res.Key)
			}

			if resource.IsSecret(res.Type) && stdoutIsTTY() && !reveal {
				fmt.Println("••••••••")
				fmt.Fprintf(os.Stderr, "(%s is a %s — use --reveal to print it, or pipe the output)\n", res.Key, res.Type)
				return nil
			}

			v, err := openVault()
			if err != nil {
				return err
			}
			plain, err := v.Decrypt(res.Key, res.Value)
			if err != nil {
				return err
			}
			os.Stdout.Write(plain)
			if stdoutIsTTY() && !strings.HasSuffix(string(plain), "\n") {
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&reveal, "reveal", "r", false, "print secret values to the terminal")
	cmd.Flags().BoolVar(&metaOnly, "meta", false, "print metadata as JSON instead of the value")
	return cmd
}
