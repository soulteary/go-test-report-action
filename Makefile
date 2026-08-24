BINARY := gtr
PKG := ./cmd/gtr
COVERPROFILE := coverage.out

.PHONY: all build test cover cover-html vet fmt fmt-check tidy lint clean smoke patch-cover

all: fmt-check vet test

build:
	go build -trimpath -o bin/$(BINARY) $(PKG)

test:
	go test ./...

# Run tests with coverage and print the total.
cover:
	go test -covermode=atomic -coverprofile=$(COVERPROFILE) ./...
	go tool cover -func=$(COVERPROFILE) | tail -n 1

cover-html: cover
	go tool cover -html=$(COVERPROFILE)

vet:
	go vet ./...

# Requires golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint:
	golangci-lint run ./...

# Enforce patch coverage (new lines >= 90%) against BASE_REF (default origin/main).
# Runs the script's built-in selftest first.
patch-cover:
	bash scripts/check-patch-coverage.sh --selftest
	go test -covermode=atomic -coverprofile=$(COVERPROFILE) ./...
	bash scripts/check-patch-coverage.sh $(COVERPROFILE)

fmt:
	gofmt -w $(shell git ls-files '*.go')

fmt-check:
	@out="$$(gofmt -l $(shell git ls-files '*.go'))"; \
	if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

tidy:
	go mod tidy

clean:
	rm -rf bin dist $(COVERPROFILE)

# Quick local smoke: run the CLI against the passing fixture.
smoke: build
	cd testdata/passing && ../../bin/$(BINARY) run \
		--coverage-threshold 80 \
		--json-output /tmp/gtr.json \
		--markdown-output /tmp/gtr.md \
		--svg-output /tmp/gtr.svg
