VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X github.com/kashportsa/kp-gruuk/internal/client.version=$(VERSION)"

.PHONY: all build build-server build-client test lint clean

all: build

build: build-server build-client

build-server:
	CGO_ENABLED=0 go build -o bin/gruuk-server $(LDFLAGS) ./cmd/gruuk-server

build-client:
	CGO_ENABLED=0 go build -o bin/gruuk $(LDFLAGS) ./cmd/gruuk

test:
	go test -race ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

# Cross-compilation targets for releases
.PHONY: release
release:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/gruuk-darwin-amd64 $(LDFLAGS) ./cmd/gruuk
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/gruuk-darwin-arm64 $(LDFLAGS) ./cmd/gruuk
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/gruuk-linux-amd64 $(LDFLAGS) ./cmd/gruuk
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/gruuk-linux-arm64 $(LDFLAGS) ./cmd/gruuk
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/gruuk-server-darwin-amd64 $(LDFLAGS) ./cmd/gruuk-server
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/gruuk-server-darwin-arm64 $(LDFLAGS) ./cmd/gruuk-server
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/gruuk-server-linux-amd64 $(LDFLAGS) ./cmd/gruuk-server
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/gruuk-server-linux-arm64 $(LDFLAGS) ./cmd/gruuk-server

.PHONY: docker
docker:
	docker build -t gruuk-server -f Dockerfile.server .
