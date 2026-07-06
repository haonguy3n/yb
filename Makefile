BIN := yb
PKG := ./cmd/yb

.PHONY: build test install clean

## build: compile the yb binary
build:
	go build -o $(BIN) $(PKG)

## test: run unit tests
test:
	go test ./...

## install: install yb onto $GOPATH/bin
install:
	go install $(PKG)

## clean: remove the binary
clean:
	rm -f $(BIN)
