BINARY := koan
PKG    := ./cmd/koan

.PHONY: build install run test fmt fix vet tidy check clean help

build:
	go build -o $(BINARY) $(PKG)

install:
	go install $(PKG)

# Usage: make run ARGS="examples/art-director/art-director.txt --seed 412"
run:
	go run $(PKG) $(ARGS)

test:
	go test ./...

fmt:
	gofmt -w .

# `go fix` converges over several passes; "re-run to apply more" is
# progress, not failure. Loop until a pass is quiet.
fix:
	go fix ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# The pre-commit sequence from AGENTS.md, in order. There is no hook
# yet; run this before every commit.
check: fix fmt vet tidy test

clean:
	rm -f $(BINARY)

help:
	@echo "Targets:"
	@echo "  build    - compile ./$(BINARY) from $(PKG)"
	@echo "  install  - go install to \$$GOBIN"
	@echo "  run      - go run (pass args via ARGS=\"...\")"
	@echo "  test     - go test ./..."
	@echo "  fmt      - gofmt -w ."
	@echo "  fix      - go fix ./..."
	@echo "  vet      - go vet ./..."
	@echo "  tidy     - go mod tidy"
	@echo "  check    - fix, fmt, vet, tidy, test: the pre-commit sequence"
	@echo "  clean    - remove the local binary"
