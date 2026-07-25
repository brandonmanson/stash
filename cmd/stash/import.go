package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"stash/internal/resource"
)

func importCmd() *cobra.Command {
	var prefix string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import <env-file>",
		Short: "Import a .env file (the plaintext problem this tool exists to end)",
		Long: `Import KEY=VALUE pairs from a .env file. Each variable becomes a resource
under the given prefix — GITHUB_TOKEN with --prefix env.myapp becomes
env.myapp.github_token — with a secret-ish type inferred from the name
(*_TOKEN → token, *_PASSWORD → password, *KEY*/*SECRET* → credential).

The source file is left untouched; delete it yourself once you've verified.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pairs, err := parseEnvFile(args[0])
			if err != nil {
				return err
			}
			if len(pairs) == 0 {
				return fmt.Errorf("no KEY=VALUE pairs found in %s", args[0])
			}

			var v interface {
				Encrypt(string, []byte) ([]byte, error)
			}
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			if !dryRun {
				vv, err := openVault()
				if err != nil {
					return err
				}
				v = vv
			}

			for _, p := range pairs {
				key := prefix + "." + strings.ToLower(p.name)
				typ := inferEnvType(p.name)
				if err := resource.ValidateKey(key); err != nil {
					fmt.Fprintf(os.Stderr, "skip %s: %v\n", p.name, err)
					continue
				}
				if dryRun {
					fmt.Printf("would store %s (%s)\n", key, typ) // resource key NAMES are non-secret by design — @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-sensitive-data-in-logs:accepted-risk:2026-10-23
					continue
				}
				enc, err := v.Encrypt(key, []byte(p.value))
				if err != nil {
					return err
				}
				dissolved, err := st.Put(resource.Resource{
					Key: key, Type: typ, Value: enc,
					Metadata: map[string]string{"source": args[0], "env_var": p.name},
				})
				if err != nil {
					return fmt.Errorf("storing %s: %w", key, err)
				}
				printDissolved(dissolved)
				fmt.Printf("stored %s (%s)\n", key, typ) // @waiver:backstop/go-standards/backstop.packs.backstop.go-standards.rules.security.go.security.no-sensitive-data-in-logs:accepted-risk:2026-10-23
			}
			if !dryRun {
				fmt.Fprintf(os.Stderr, "\nimported %d resources from %s — the file is untouched; remove it when ready\n", len(pairs), args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "env", "namespace prefix for imported keys")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be imported without storing")
	return cmd
}

type envPair struct{ name, value string }

func parseEnvFile(path string) ([]envPair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening env file: %w", err)
	}
	defer f.Close()
	var out []envPair
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		val = strings.TrimSpace(val)
		// Strip one layer of matching quotes.
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' || val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		if name == "" || val == "" {
			continue
		}
		out = append(out, envPair{name, val})
	}
	return out, sc.Err()
}

func inferEnvType(name string) string {
	n := strings.ToUpper(name)
	switch {
	case strings.HasSuffix(n, "_TOKEN") || n == "TOKEN":
		return resource.TypeToken
	case strings.Contains(n, "PASSWORD") || strings.HasSuffix(n, "_PASS"):
		return resource.TypePassword
	case strings.Contains(n, "SECRET") || strings.Contains(n, "KEY") || strings.Contains(n, "CREDENTIAL"):
		return resource.TypeCredential
	case strings.Contains(n, "URL") || strings.Contains(n, "URI") || strings.Contains(n, "HOST") || strings.Contains(n, "ENDPOINT"):
		return resource.TypeEndpoint
	default:
		return resource.TypeNote
	}
}
