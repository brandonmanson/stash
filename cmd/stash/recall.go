package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/brandonmanson/stash/internal/embed"
	"github.com/brandonmanson/stash/internal/resource"
	"github.com/brandonmanson/stash/internal/store"
)

func recallCmd() *cobra.Command {
	var around string
	var limit int
	cmd := &cobra.Command{
		Use:   "recall <vague description>",
		Short: "Find a resource from a vague memory of it (local semantic search over descriptions)",
		Long: `Recall a resource you only vaguely remember — "the service that renegotiated
my water bill" — by semantic similarity over descriptions, keys, tags, and
types. Values are never embedded or searched.

--around narrows to a time window (±6 weeks) when you remember roughly when:
  stash recall "water bill negotiation" --around 2024-10

Runs fully locally: the first use downloads a small embedding model to
~/.stash/models; nothing you store or search ever leaves the machine.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			entries, err := st.List("")
			if err != nil {
				return err
			}
			if around != "" {
				entries, err = filterAround(entries, around)
				if err != nil {
					return err
				}
			}
			if len(entries) == 0 {
				fmt.Fprintln(os.Stderr, "nothing to recall in that window")
				return nil
			}

			home, err := stashHome()
			if err != nil {
				return err
			}
			emb, model, err := embed.Open(home)
			if err != nil {
				return err
			}
			defer emb.Close()

			vectors, err := ensureEmbeddings(st, emb, model, entries)
			if err != nil {
				return err
			}
			qvec, err := emb.Embed(model.QueryPrefix + query)
			if err != nil {
				return err
			}

			type hit struct {
				entry resource.Entry
				score float64
			}
			var hits []hit
			ql := strings.ToLower(query)
			for _, e := range entries {
				v, ok := vectors[e.Key]
				if !ok {
					continue
				}
				score := embed.Cosine(qvec, v.Vector)
				// Exact memory still beats vibes: substring hits get a boost.
				if strings.Contains(strings.ToLower(embed.Text(e)), ql) {
					score += 0.15
				}
				hits = append(hits, hit{e, score})
			}
			sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
			if len(hits) > limit {
				hits = hits[:limit]
			}

			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			for _, h := range hits {
				desc := h.entry.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				fmt.Fprintf(w, "%.3f\t%s\t[%s]\t%s\n", h.score, h.entry.Key, h.entry.Type, desc)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&around, "around", "", "narrow to ±6 weeks of a date (2024-10 or 2024-10-15)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "max results")
	return cmd
}

// ensureEmbeddings lazily (re)embeds entries whose stored vector is missing
// or stale for the active model, so recall needs no separate index step.
func ensureEmbeddings(st store.Store, emb embed.Embedder, model embed.Model, entries []resource.Entry) (map[string]store.Embedding, error) {
	stored, err := st.ListEmbeddings(model.Name)
	if err != nil {
		return nil, fmt.Errorf("loading stored embeddings: %w", err)
	}
	var pending []resource.Entry
	for _, e := range entries {
		text := embed.Text(e)
		if s, ok := stored[e.Key]; !ok || s.TextHash != embed.Hash(text, model.Name) {
			pending = append(pending, e)
		}
	}
	if len(pending) > 0 {
		fmt.Fprintf(os.Stderr, "indexing %d resource(s)...\n", len(pending))
		for _, e := range pending {
			text := embed.Text(e)
			vec, err := emb.Embed(model.DocPrefix + text)
			if err != nil {
				return nil, fmt.Errorf("embedding %s: %w", e.Key, err)
			}
			s := store.Embedding{Key: e.Key, Model: model.Name, Dim: len(vec), Vector: vec, TextHash: embed.Hash(text, model.Name)}
			if err := st.PutEmbedding(s); err != nil {
				return nil, fmt.Errorf("saving embedding for %s: %w", e.Key, err)
			}
			stored[e.Key] = s
		}
	}
	return stored, nil
}

func filterAround(entries []resource.Entry, around string) ([]resource.Entry, error) {
	var center time.Time
	var err error
	for _, layout := range []string{"2006-01", "2006-01-02"} {
		if center, err = time.Parse(layout, around); err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("--around wants 2024-10 or 2024-10-15, got %q", around)
	}
	const window = 42 * 24 * time.Hour // ±6 weeks
	var out []resource.Entry
	for _, e := range entries {
		if d := e.CreatedAt.Sub(center); d > -window && d < window {
			out = append(out, e)
		}
	}
	return out, nil
}
