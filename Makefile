PREFIX ?= $(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet clean install

build:
	go build -ldflags "-X main.version=$(VERSION)" -o trimout .

test:
	go test -v ./...

vet:
	go vet ./...

install: build
	mkdir -p $(PREFIX)
	cp trimout $(PREFIX)/trimout

clean:
	rm -f trimout
