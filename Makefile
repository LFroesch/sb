BIN := sb
FOREMAN_BIN := sb-foreman
BUILD_TARGET := .
FOREMAN_BUILD_TARGET := ./cmd/foreman
INSTALL_DIR ?= $(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) $(BUILD_TARGET)
	go build -ldflags "-s -w" -o $(FOREMAN_BIN) $(FOREMAN_BUILD_TARGET)

install: build
	mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BIN) $(INSTALL_DIR)/$(BIN)
	install -m 0755 $(FOREMAN_BIN) $(INSTALL_DIR)/$(FOREMAN_BIN)

.PHONY: build install
