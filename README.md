# stash

An encrypted local resource store with a filesystem-shaped CLI. Everything is
a resource — API keys, passwords, links, endpoints, notes, dates — addressed
by dot-namespaces, encrypted at rest, master key in the macOS Keychain.

```
go build -o bin/stash ./cmd/stash
stash init

stash put credentials.github.work            # hidden prompt (secret type inferred)
stash put links.mongodb.prod 'https://...'   # link type inferred
stash get credentials.github.work            # masked on a terminal
stash get credentials.github.work --reveal   # ...unless you ask
export GH_TOKEN=$(stash get credentials.github.work)   # piped = raw, always

stash ls                    # namespace tree
stash ls credentials        # subtree
stash search mongo          # keys, types, tags, metadata
stash import .env --prefix env.myapp --dry-run
stash rm notes.scratch

# Reserve structure before you have values — reserved keys are a checklist
# (`ls --unfilled`), participate in collision rules, and fill via plain put:
stash reserve acme.resend.credentials.key
stash reserve agency.engagements.acme --like agency.engagements.oldclient  # stamp a shape

# Descriptions are written for future-you; recall finds them from vibes:
stash put subs.billshark X --description "renegotiated my municipal water bill"
stash recall "that water bill negotiation service" --around 2024-10
```

`recall` is fully local semantic search: build with `make build` (statically
links llama.cpp), and the first use downloads a small GGUF embedding model to
`~/.stash/models`. Only descriptions, keys, tags, and types are embedded —
never values. Plain `go build` works too; `recall` then tells you how to get
the full build.

Shell completion (`stash completion zsh`) tab-completes resource keys one
namespace segment at a time, without unlocking the store.

## Security model (MVP)

- Values encrypted per-resource with AES-256-GCM; the resource key is bound in
  as AAD so ciphertexts can't be swapped between resources.
- A random data key encrypts values; a random KEK wraps the data key; the KEK
  lives in the macOS Keychain (`dev.stash`). `keys.json` holds only the
  wrapped data key.
- Keys/types/tags/metadata are **plaintext at rest** (queryable without
  unlock); values never are.
- Secret types (credential, password, token, certificate) mask on a terminal
  and prompt hidden on entry.

`stash use` is reserved: `get` transfers *custody* (you hold the plaintext);
`use` will transfer *authority* (stash performs the operation, the secret is
never revealed). It lands with the `stashd` daemon. See `DECISIONS.md` for
what this MVP deliberately defers.
