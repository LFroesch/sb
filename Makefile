BIN := sb
FOREMAN_BIN := sb-foreman
BUILD_TARGET := .
FOREMAN_BUILD_TARGET := ./cmd/foreman
INSTALL_DIR ?= $(HOME)/.local/bin
VERSION ?= $(shell sh -c 'tag=$$(git tag --points-at HEAD | sort -V | tail -n1); if [ -n "$$tag" ]; then printf "%s" "$$tag"; else git describe --tags --always --dirty 2>/dev/null || echo dev; fi')

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) $(BUILD_TARGET)
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(FOREMAN_BIN) $(FOREMAN_BUILD_TARGET)

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN) $(INSTALL_DIR)/$(BIN)
	install -m 0755 $(FOREMAN_BIN) $(INSTALL_DIR)/$(FOREMAN_BIN)

.PHONY: build install
