.PHONY: all generate proto web test build run clean iso deploy

export PATH := $(HOME)/go/bin:$(PATH)

-include .env
-include .env.deploy

LIVE_HOST ?= $(NASCTL_DEPLOY_HOST)
LIVE_BIN ?= $(if $(NASCTL_DEPLOY_BIN),$(NASCTL_DEPLOY_BIN),/root/nasctl)
LIVE_SERVICE ?= $(if $(NASCTL_DEPLOY_SERVICE),$(NASCTL_DEPLOY_SERVICE),nasctl)

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

deploy: build
	@if [ -z "$(LIVE_HOST)" ]; then echo "Error: LIVE_HOST or NASCTL_DEPLOY_HOST is not set. Provide in .env.deploy or environment."; exit 1; fi
	objcopy --remove-section .note.gnu.property bin/nasctl 2>/dev/null || true
	scp bin/nasctl $(LIVE_HOST):$(LIVE_BIN).new
	ssh $(LIVE_HOST) "mv $(LIVE_BIN).new $(LIVE_BIN) && systemctl restart $(LIVE_SERVICE) && systemctl is-active $(LIVE_SERVICE)"

clean:
	rm -rf bin/ web/dist

