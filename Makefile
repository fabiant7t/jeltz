BINARY      := jeltz
CMD         := ./cmd/jeltz
GOFLAGS     :=
CGO_ENABLED := 0

VERSION    := dev
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_REV    := $(shell git rev-parse --short HEAD 2>/dev/null)

LDFLAGS := -X main.version=$(VERSION) \
           -X main.buildDate=$(BUILD_DATE) \
           -X main.gitRevision=$(GIT_REV)

.PHONY: all build test race lint clean

all: build

build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

test:
	go test ./... -timeout 120s

race:
	go test -race ./... -timeout 120s

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
