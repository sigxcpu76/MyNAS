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

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.5.0-dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "HEAD")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%d')
LDFLAGS := -s -w -X 'github.com/sigxcpu/nas/internal/version.Version=$(VERSION)' -X 'github.com/sigxcpu/nas/internal/version.GitCommit=$(COMMIT)' -X 'github.com/sigxcpu/nas/internal/version.BuildDate=$(BUILD_DATE)'

build: web
	go build -ldflags="$(LDFLAGS)" -o bin/nasctl ./cmd/nasctl
	objcopy --remove-section .note.gnu.property bin/nasctl 2>/dev/null || true

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

deploy-all: build
	@echo "Deploying to nasulica (192.168.27.23) and library (192.168.27.25)..."
	scp bin/nasctl root@192.168.27.23:/usr/local/bin/nasctl.new
	ssh root@192.168.27.23 "mv /usr/local/bin/nasctl.new /usr/local/bin/nasctl && systemctl restart nasctl"
	scp bin/nasctl root@192.168.27.25:/usr/local/bin/nasctl.new
	ssh root@192.168.27.25 "mv /usr/local/bin/nasctl.new /usr/local/bin/nasctl && systemctl restart nasctl"
	@echo "Deployment complete on all appliances."

clean:
	rm -rf bin/ web/dist

