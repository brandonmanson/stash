package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"stash/internal/resource"
)

func putCmd() *cobra.Command {
	var typ, description string
	var tags, metas []string
	cmd := &cobra.Command{
		Use:   "put <key> [value]",
		Short: "Store a resource (value from arg, stdin, or an interactive prompt)",
		Long: `Store a resource. The value comes from the argument, from piped stdin,
or from an interactive prompt (hidden for secret types).

The type is inferred from the first type-ish segment anywhere in the key —
resend.credentials.key is a credential, jason.birthday is a date, and
credentials/tokens/passwords/certs segments mark secret types wherever they
appear. Anything else defaults to note. Override with --type.`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: keyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if err := resource.ValidateKey(key); err != nil {
				return err
			}

			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			// On an existing key (reserved or filled), preserve its type,
			// tags, and metadata unless flags override them.
			verb := "stored"
			meta := map[string]string{}
			if existing, err := st.Get(key); err == nil {
				if typ == "" {
					typ = existing.Type
				}
				if len(tags) == 0 {
					tags = existing.Tags
				}
				if len(metas) == 0 && existing.Metadata != nil {
					meta = existing.Metadata
				}
				if description == "" {
					description = existing.Description
				}
				if existing.Reserved {
					verb = "filled reservation"
				} else {
					verb = "updated"
				}
			}
			if typ == "" {
				typ = resource.InferType(key)
			}

			value, err := readValue(args, typ)
			if err != nil {
				return err
			}
			if len(value) == 0 {
				return fmt.Errorf("empty value — nothing stored")
			}

			for _, m := range metas {
				k, v, ok := strings.Cut(m, "=")
				if !ok {
					return fmt.Errorf("--meta wants key=value, got %q", m)
				}
				meta[k] = v
			}

			v, err := openVault()
			if err != nil {
				return err
			}
			enc, err := v.Encrypt(key, value)
			if err != nil {
				return err
			}
			dissolved, err := st.Put(resource.Resource{
				Key: key, Type: typ, Value: enc, Metadata: meta, Tags: tags,
				Description: description,
			})
			if err != nil {
				return err
			}
			for _, d := range dissolved {
				fmt.Fprintf(os.Stderr, "note: reservation %s dissolved into a namespace\n", d)
			}
			fmt.Fprintf(os.Stderr, "%s %s (%s)\n", verb, key, typ)
			return nil
		},
	}
	cmd.Flags().StringVarP(&typ, "type", "t", "", "resource type (default inferred from namespace)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().StringArrayVarP(&metas, "meta", "m", nil, "metadata key=value (repeatable)")
	cmd.Flags().StringVarP(&description, "description", "d", "", "free-text description, written to be found by future-you (drives recall)")
	cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions([]string{
		resource.TypeCredential, resource.TypePassword, resource.TypeToken,
		resource.TypeCertificate, resource.TypeNote, resource.TypeLink,
		resource.TypeEndpoint, resource.TypeDate, resource.TypeBlob,
	}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func readValue(args []string, typ string) ([]byte, error) {
	if len(args) == 2 {
		if resource.IsSecret(typ) {
			fmt.Fprintln(os.Stderr, "warning: secret passed as a command argument is visible in shell history and `ps` — prefer the interactive prompt (omit the value) or stdin (`... | stash put <key>`)")
		}
		return []byte(args[1]), nil
	}
	if !stdinIsTTY() {
		return io.ReadAll(os.Stdin)
	}
	// Interactive prompt; hidden entry for secret types.
	if resource.IsSecret(typ) {
		fmt.Fprintf(os.Stderr, "value (%s, hidden): ", typ)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return b, err
	}
	fmt.Fprintf(os.Stderr, "value (%s): ", typ)
	var line string
	_, err := fmt.Scanln(&line)
	if err != nil && err.Error() != "unexpected newline" {
		return nil, err
	}
	return []byte(line), nil
}
