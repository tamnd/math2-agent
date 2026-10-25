# The build writes to bin/. Git ignores bin/.
BINARY  := bin/m2
PKG     := ./cmd/m2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: build install test vet fmt lint vuln clean

build:
	@mkdir -p $(dir $(BINARY))
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test -race -shuffle=on ./...

vet:
	go vet ./...

fmt:
	gofmt -w -s .

lint:
	golangci-lint run

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

clean:
	rm -rf bin dist coverage.out
