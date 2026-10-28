.PHONY: all iso iso-docker iso-local clean

REPO_ROOT := $(shell cd .. && pwd)
IMAGE_NAME := mynas-iso-builder:latest
COMP ?= lz4

all: iso

iso: iso-docker

iso-docker:
	@echo ">> Building ISO Builder container image..."
	docker build -f Dockerfile.builder -t $(IMAGE_NAME) .
	@echo ">> Executing appliance ISO build inside container (compression: $(COMP))..."
	docker run --rm --privileged \
		-e SQUASHFS_COMP=$(COMP) \
		-v $(REPO_ROOT):/workspace \
		$(IMAGE_NAME)

iso-local:
	@echo ">> Running local ISO build script (compression: $(COMP))..."
	SQUASHFS_COMP=$(COMP) ./build.sh

clean:
	@echo ">> Cleaning ISO build artifacts..."
	rm -rf build/ output/
