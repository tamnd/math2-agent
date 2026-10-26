# The build writes to bin/. Git ignores bin/.
BINARY  := bin/m2
PKG     := ./cmd/m2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: build install test vet fmt lint vuln proto clean

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

# The table of methods in api/methods.go makes api/m2.proto, and buf makes the Go code in api/m2pb from it. buf, protoc-gen-go and protoc-gen-go-grpc must be in the PATH.
proto:
	go test ./api -run '^TestProto$$' -update
	cd api && buf lint && buf generate --path m2.proto

clean:
	rm -rf bin dist coverage.out
