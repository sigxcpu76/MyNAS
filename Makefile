.PHONY: all generate proto web test build run clean iso

export PATH := $(HOME)/go/bin:$(PATH)

all: generate

generate:
	go run generate.go

proto:
	buf generate

web:
	cd web && npm run build

test:
	go test -v -race ./cmd/... ./internal/...
	cd web && npm test -- --run

build: web
	go build -o bin/nasctl ./cmd/nasctl

COMP ?= lz4

iso: build
	$(MAKE) -C iso iso COMP=$(COMP)

run: build
	./bin/nasctl start --http-port 8080 --grpc-port 9090

clean:
	rm -rf bin/ web/dist

