// stash — an encrypted local resource store with a filesystem-shaped CLI.
//
// MVP wiring is CLI-direct: commands open the store and vault in-process.
// The Store interface is the seam where stashd (DD-4) slots in later.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"stash/internal/store"
	"stash/internal/vault"
)

func stashHome() (string, error) {
	if h := os.Getenv("STASH_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".stash"), nil
}

func openStore() (store.Store, error) {
	home, err := stashHome()
	if err != nil {
		return nil, fmt.Errorf("locating stash home: %w", err)
	}
	if _, err := os.Stat(filepath.Join(home, "keys.json")); err != nil {
		return nil, fmt.Errorf("stash is not initialized — run `stash init`")
	}
	return store.OpenSQLite(filepath.Join(home, "stash.db"))
}

func openVault() (*vault.Vault, error) {
	home, err := stashHome()
	if err != nil {
		return nil, fmt.Errorf("locating stash home: %w", err)
	}
	return vault.Open(home)
}

func stdinIsTTY() bool  { return term.IsTerminal(int(os.Stdin.Fd())) }
func stdoutIsTTY() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

// keyCompletion tab-completes resource keys one namespace segment at a time,
// filesystem-style. Metadata is plaintext at rest, so this needs no unlock.
func keyCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	st, err := openStore()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer st.Close()
	entries, err := st.List("")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Key, toComplete) {
			continue
		}
		// Complete up to the next dot past the typed portion, if any.
		cand := e.Key
		if idx := strings.Index(e.Key[len(toComplete):], "."); idx >= 0 {
			cand = e.Key[:len(toComplete)+idx+1]
		}
		if !seen[cand] {
			seen[cand] = true
			out = append(out, cand)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
}

func main() {
	root := &cobra.Command{
		Use:           "stash",
		Short:         "Encrypted local resource store",
		Long:          "stash — an encrypted local store for everything a developer keeps at hand:\nAPI keys, passwords, links, endpoints, notes. Filesystem-shaped, vault-safe.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		initCmd(), putCmd(), getCmd(), lsCmd(), searchCmd(), recallCmd(), rmCmd(), reserveCmd(), importCmd(), useCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "stash:", err)
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the stash: create the store and anchor the master key in the macOS Keychain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := stashHome()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(home, 0o700); err != nil {
				return fmt.Errorf("creating stash home: %w", err)
			}
			if err := vault.Init(home); err != nil {
				return err
			}
			st, err := store.OpenSQLite(filepath.Join(home, "stash.db"))
			if err != nil {
				return err
			}
			st.Close()
			fmt.Printf("Initialized stash in %s\n", home)
			fmt.Println("Master key stored in the macOS Keychain (service: dev.stash).")
			fmt.Println("\nTry:  stash put credentials.github.work   # prompts for the secret")
			return nil
		},
	}
}
