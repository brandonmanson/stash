//go:build !llama

package embed

import "fmt"

func newBackend(modelPath string) (Embedder, error) {
	return nil, fmt.Errorf("this stash binary was built without the embedding backend — build with `make build` (llama.cpp statically linked, build tag `llama`) to enable recall")
}
