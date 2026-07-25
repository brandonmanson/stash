# MVP Decisions — provisional, not bundle resolutions

Source: `backstop-core/bundles/BUNDLE-018-stash-local-resource-store.bundle.md`
(maturity: `exploring`). The bundle's OQs remain **OPEN** — this file records
what the MVP *provisionally* adopted so the bundle can resolve or overturn each
with full context. Decided with the founder on 2026-07-24.

> Bundle/OQ/DD references point into backstop, the (currently private)
> agent-discipline framework this project's design process ran under. The
> references are kept as provenance; the decisions below stand on their own.

| OQ | MVP choice | Notes |
|----|-----------|-------|
| OQ-1 home/runtime | Own repo (this one), Go | Matches lean. |
| OQ-2 storage/encryption | SQLite (`modernc.org/sqlite`); per-resource AES-256-GCM with resource key as AAD; keychain-wrapped data key | Matches lean. **Keys, types, tags, metadata are plaintext at rest** (enables ls/search/completion without unlock) — only values are encrypted. Revisit if metadata is deemed sensitive. |
| OQ-3 key custody | macOS Keychain only (KEK via `/usr/bin/security`); no passphrase fallback, no lock/timeout semantics | Founder call: don't enshrine a weak headless default. Headless/Linux/recovery still open. Known limit: the keychain item is global (`dev.stash`/`kek`) — a second `stash init` with a different `STASH_HOME` overwrites the KEK and bricks the first store. |
| OQ-4 caller identity | **Moot for MVP** — no daemon, so the caller *is* the user's process | Still the load-bearing OQ for the daemon phase. |
| OQ-5 policy + `use` scope | Seam only. Policy column exists (JSON, default `{}`) but no engine; `use` is a registered command that explains itself and exits nonzero | Masking behavior is driven by *type*, not policy, in the MVP. |
| OQ-6 namespaces | Real hierarchy: subtree `ls`, segment-wise completion, leaf-XOR-namespace collision rule enforced on `put` (both directions) | Wildcards deferred. |
| OQ-7 types | Built-ins (credential, password, token, certificate, note, link, endpoint, date, blob) + freeform; secret types mask on TTY and prompt hidden; type inferred from top namespace segment | Matches lean (c). |
| OQ-8 capability contract | Nothing shipped; seam is the `Store` interface + reserved `use` op | Design against BUNDLE-015 later. |
| OQ-9 IPC protocol | **Deferred entirely** — MVP is CLI-direct | See architecture note below. |
| OQ-10 audit log | **Cut from MVP** (founder call) | Natural to add when the daemon owns all access. |
| OQ-11 onboarding | `.env` import with `--prefix`, `--dry-run`, name-based type inference; source file left untouched | Matches lean. |

## Post-MVP additions (founder-decided, 2026-07-24)

- **Type inference scans any segment** (not just the first): namespaces are
  entity-first in real usage (`resend.credentials.key`, `jason.birthday`).
- **`stash put` on an existing key preserves type/tags/metadata** unless
  flags override — an update is not a re-creation.
- **`reserve`**: declare a leaf (key + type) before its value exists. Scope
  chosen: per-key + `--like <subtree>` shape-stamping (named templates
  deliberately deferred — an existing subtree is the template). Reserved keys
  participate in collision rules, render as `(reserved)` in `ls` (filter:
  `--unfilled`), refuse `get` with a fill hint, need no unlock, and fill via
  plain `put`. Forward-relevant to OQ-8: a reservation is a declared
  dependency — the shape a capability slot takes before it's funded.
- **Lazy resolution of reservations** (founder-decided over "declared
  namespace" trailing-dot syntax): a reservation is *unresolved intent*, and
  the first concrete action decides it — `put` AT it fills it as a leaf;
  `put`/`reserve` UNDER it dissolves it into a namespace (stderr notice).
  Dissolution flows downward only: a coarse `put` never destroys a deeper
  reservation (specific beats coarse), and filled leaves never dissolve.
  Enables `reserve agency.engagements.<client>` as a placeholder that later
  deep puts (human or scripted) refine into real structure.

## Recall / semantic retrieval (founder-decided, 2026-07-25)

Solves recall-by-vibes: write-time-you knows everything, read-time-you
remembers a season and what the thing did. Decisions:

- **`--description` (-d) on put/reserve** — free text written to be found by
  future-you. First-class column, plain-searchable, preserved on update.
  (Named `--description`, not `--desc` — founder call, SQL `DESC` collision.)
- **Embeddings are derived only from non-secret metadata** (description, key
  words, type, tags) — the embedding path receives Entry, not Resource, so
  values structurally cannot reach it. This is what makes local semantic
  search compatible with the encryption story: DD-1's no-embeddings non-goal
  was about not becoming a PKM platform and about custody leaks; embedding
  deliberately-written descriptions with a local model does neither.
- **Inference baked into the CLI** behind the `llama` build tag: statically
  linked llama.cpp (tcpipuk/llama-go binding, vendored clone in .deps/ via
  Makefile). Plain `go build` yields a binary whose `recall` explains how to
  get the full build. No sidecar, no service, no dylib.
- **Model = runtime config, not compile-time commitment**: GGUF fetched once
  to `~/.stash/models` on first use (announced, not hidden). Registry:
  bge-small-en-v1.5 (default, 34M params) and nomic-embed-text-v1.5;
  `$STASH_EMBED_MODEL` selects. voyage-4-nano's community GGUF was evaluated
  but ships a detached linear-projection head — excluded until official.
- **Vectors in an `embeddings` side table** stamped with model+dim+text-hash:
  different embedding spaces are never silently compared; stale or missing
  vectors re-embed lazily at recall time (no separate index step); `rm`
  cascades.
- **Hybrid scoring**: optional `--around <month>` time filter (±6 weeks,
  vague memories are temporally anchored) → cosine rank → +0.15 boost for
  plain substring hits, so exact memory still beats vibes.

## Backstop onboarding — gates only (founder-decided, 2026-07-25)

Onboarded to backstop solely for enforcement gates: `backstop.yml` declares
the `backstop/go-standards` pack (local); `test_verification` is set to
`level: off` because there are no spec artifacts — the artifact lifecycle is
deliberately NOT adopted yet. Gate is green. Notable calls from the first
sweep (46 violations → 0):

- **Refactored to comply (32)**: all bare `return err` sites wrapped with
  context; two ignored errors handled; six package-level `var`s eliminated
  (lookup maps → switch functions, model registry → `Registry()`, sentinel
  `ErrNotFound` → typed `NotFoundError`); `sqliteStore.db` field renamed
  `conn` (the pack's own valid fixture establishes non-regex-matching field
  names as the constructor-injection pattern).
- **Waived as false positives (9, expire 2026-10-23)**: resource *type name*
  consts (`"password"`, `"token"`), keychain item *coordinates*
  (`dev.stash`/`kek`), `keys.json` filename, and four prints of resource
  *key names* — a secrets store's own vocabulary trips credential/log rules.
  Pack-improvement candidates (e.g. exempt consts, entropy heuristics)
  belong in backstop-go-pack, not here.

## Architecture: conscious, temporary deviation from DD-4

DD-4 says the CLI never touches storage — a thin CLI talks to `stashd` over
IPC. The MVP deliberately ships **CLI-direct** (founder call, 2026-07-24) to
get to working software fastest. The seam is `internal/store.Store`: when
`stashd` lands, an IPC-backed implementation of that interface fronts the
SQLite one and the CLI commands don't change. Until then there is no daemon,
so **launchd packaging is parked** with it (nothing to install), and caller
identity / audit / `use` are all blocked behind the daemon phase — which is
the natural next milestone.
