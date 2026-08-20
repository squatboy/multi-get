GO ?= go
BINARY ?= bin/kubectl-multi

.PHONY: build test vet e2e-local

build:
	mkdir -p bin
	$(GO) build -o $(BINARY) ./cmd/kubectl-multi

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

e2e-local:
	./hack/e2e-local.sh
