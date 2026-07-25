# `make build` produces bin/stash with the embedding backend compiled in
# (llama.cpp statically linked, build tag `llama`). Plain `go build` still
# works and yields a binary where `recall` explains how to get the full one.

DEPS := .deps/llama-go

.PHONY: build test clean

build: $(DEPS)/libbinding.a
	LIBRARY_PATH=$(CURDIR)/$(DEPS) C_INCLUDE_PATH=$(CURDIR)/$(DEPS) \
		go build -tags llama -o bin/stash ./cmd/stash

$(DEPS)/libbinding.a: $(DEPS)
	cd $(DEPS) && $(MAKE) libbinding.a
	cd $(DEPS) && find build -name '*.a' -exec cp {} . \;

$(DEPS):
	mkdir -p .deps
	git clone --recurse-submodules --depth 1 --shallow-submodules \
		https://github.com/tcpipuk/llama-go $(DEPS)

test:
	go test ./...

clean:
	rm -rf bin
