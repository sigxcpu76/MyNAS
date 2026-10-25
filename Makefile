.PHONY: all proto web test build run clean

export PATH := $(HOME)/go/bin:$(PATH)

all: proto web test build

proto:
	buf generate

web:
	cd web && npm run build

test:
	go test -v -race ./...
	cd web && npm test -- --run

build: web
	go build -o bin/nasctl ./cmd/nasctl

run: build
	./bin/nasctl start --http-port 8080 --grpc-port 9090

clean:
	rm -rf bin/ web/dist
