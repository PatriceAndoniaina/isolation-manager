.PHONY: help build test race coverage lint fmt vet tidy clean

BIN := bin/isolation-manager
PKG := ./src/...

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

coverage:
	go test -coverprofile=coverage.out $(PKG)
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
