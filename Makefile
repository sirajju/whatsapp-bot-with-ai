BINARY := wabot
BIN_DIR := bin
GOFLAGS := -trimpath -ldflags="-s -w"

.PHONY: build build-linux run tidy clean fmt vet

build:
	mkdir -p $(BIN_DIR)
	go build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/wabot

# For your local server (amd64 Linux). No cgo thanks to modernc.org/sqlite.
build-linux:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY)-linux-amd64 ./cmd/wabot

run:
	go run ./cmd/wabot

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf $(BIN_DIR) dist
