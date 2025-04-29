LINT_VERSION := v2.1.5

ifeq ($(RACE),1)
	GOFLAGS+=-race
endif
PKG := `go list ${GOFLAGS} -f {{.Dir}} ./...`

tools:
	@curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin ${LINT_VERSION}

fmt:
	@golangci-lint fmt

lint:
	@golangci-lint version
	@golangci-lint config verify
	@golangci-lint run

run:
	@echo "Compiling"
	@go run $(GOFLAGS) examples/main.go

test:
	@echo "Running tests"
	@go test -count=1 $(GOFLAGS) -coverprofile=coverage.txt -covermode count ./...

