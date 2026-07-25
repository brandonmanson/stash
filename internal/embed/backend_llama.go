//go:build llama

package embed

import (
	"fmt"
	"os"

	llama "github.com/tcpipuk/llama-go"
)

// llamaBackend runs GGUF embedding models via statically-linked llama.cpp.
// Post-it-note-sized inputs make CPU inference plenty fast.
type llamaBackend struct {
	model *llama.Model
	ctx   *llama.Context
}

func newBackend(modelPath string) (Embedder, error) {
	// llama.cpp is chatty on stderr by default; errors-only unless the user
	// asks for more via LLAMA_LOG.
	if os.Getenv("LLAMA_LOG") == "" {
		os.Setenv("LLAMA_LOG", "error")
	}
	llama.InitLogging()
	m, err := llama.LoadModel(modelPath, llama.WithSilentLoading())
	if err != nil {
		return nil, fmt.Errorf("loading GGUF model: %w", err)
	}
	// F16 KV cache: the binding's default quantized cache requires flash
	// attention, which small BERT-style embedding models don't use.
	ctx, err := m.NewContext(llama.WithEmbeddings(), llama.WithKVCacheType("f16"))
	if err != nil {
		m.Close()
		return nil, err
	}
	return &llamaBackend{model: m, ctx: ctx}, nil
}

func (b *llamaBackend) Embed(text string) ([]float32, error) {
	return b.ctx.GetEmbeddings(text)
}

func (b *llamaBackend) Close() {
	b.ctx.Close()
	b.model.Close()
}
