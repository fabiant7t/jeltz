BINARY  := jeltz
CMD     := ./cmd/jeltz
GOFLAGS :=

.PHONY: all build test race lint clean

all: build

build:
	go build $(GOFLAGS) -o $(BINARY) $(CMD)

test:
	go test ./... -timeout 120s

race:
	go test -race ./... -timeout 120s

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
