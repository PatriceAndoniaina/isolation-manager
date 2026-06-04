.PHONY: help build test race coverage lint fmt vet tidy clean

BIN := bin/isolation-manager
PKG := ./src/...
# Couverture mesurée sur les packages métier (règle d'archi : >80% sur pkg/).
COVERPKG := ./src/pkg/...

help:
	@echo "Available targets:"
	@echo "  make build     - Compile binary -> $(BIN)"
	@echo "  make test      - Run tests"
	@echo "  make race      - Run tests with race detector"
	@echo "  make coverage  - Generate HTML coverage report"
	@echo "  make lint      - Run golangci-lint"
	@echo "  make fmt       - Format code (gofmt + goimports)"
	@echo "  make vet       - Run go vet"
	@echo "  make tidy      - Sync go.mod/go.sum"
	@echo "  make clean     - Remove build artifacts"

build:
	go build -o $(BIN) ./src/cmd

test:
	go test -v $(PKG)

race:
	go test -race $(PKG)

# -coverpkg attribue correctement la couverture inter-packages et fusionne le
# profil : sans lui, le mode multi-packages de `go test -cover` sous-estime
# certains packages (artefact de calcul). Le total est lu via `cover -func`.
coverage:
	go test -coverpkg=$(COVERPKG) -coverprofile=coverage.out $(COVERPKG)
	@go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out

lint:
	golangci-lint run $(PKG)

fmt:
	go fmt $(PKG)
	@command -v goimports >/dev/null 2>&1 && goimports -w src || echo "goimports not installed, skipping"

vet:
	go vet $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out
