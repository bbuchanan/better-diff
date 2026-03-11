APP_NAME := better-diff
MAIN_PKG := ./cmd/better-diff
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

DIST_DIR := dist
BIN_DIR := bin
MACOS_ARM64_DIR := $(DIST_DIR)/$(APP_NAME)_darwin_arm64
MACOS_AMD64_DIR := $(DIST_DIR)/$(APP_NAME)_darwin_amd64
LINUX_AMD64_DIR := $(DIST_DIR)/$(APP_NAME)_linux_amd64
LINUX_ARM64_DIR := $(DIST_DIR)/$(APP_NAME)_linux_arm64
WINDOWS_AMD64_DIR := $(DIST_DIR)/$(APP_NAME)_windows_amd64
MACOS_ARM64_ARCHIVE := $(DIST_DIR)/$(APP_NAME)_darwin_arm64.tar.gz
MACOS_AMD64_ARCHIVE := $(DIST_DIR)/$(APP_NAME)_darwin_amd64.tar.gz
LINUX_AMD64_ARCHIVE := $(DIST_DIR)/$(APP_NAME)_linux_amd64.tar.gz
LINUX_ARM64_ARCHIVE := $(DIST_DIR)/$(APP_NAME)_linux_arm64.tar.gz
WINDOWS_AMD64_ARCHIVE := $(DIST_DIR)/$(APP_NAME)_windows_amd64.zip

.PHONY: test build build-mac build-all dist-mac dist-all install clean

test:
	go test ./...

build:
	mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) $(MAIN_PKG)

build-mac:
	mkdir -p $(MACOS_ARM64_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(MACOS_ARM64_DIR)/$(APP_NAME) $(MAIN_PKG)

build-all:
	mkdir -p $(MACOS_ARM64_DIR) $(MACOS_AMD64_DIR) $(LINUX_AMD64_DIR) $(LINUX_ARM64_DIR) $(WINDOWS_AMD64_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(MACOS_ARM64_DIR)/$(APP_NAME) $(MAIN_PKG)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(MACOS_AMD64_DIR)/$(APP_NAME) $(MAIN_PKG)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(LINUX_AMD64_DIR)/$(APP_NAME) $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(LINUX_ARM64_DIR)/$(APP_NAME) $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(WINDOWS_AMD64_DIR)/$(APP_NAME).exe $(MAIN_PKG)

dist-mac: build-mac
	cp README.md $(MACOS_ARM64_DIR)/README.md
	cp scripts/install.sh $(MACOS_ARM64_DIR)/install.sh
	chmod +x $(MACOS_ARM64_DIR)/install.sh
	tar -C $(DIST_DIR) -czf $(MACOS_ARM64_ARCHIVE) $(APP_NAME)_darwin_arm64

dist-all: build-all
	cp README.md $(MACOS_ARM64_DIR)/README.md
	cp scripts/install.sh $(MACOS_ARM64_DIR)/install.sh
	chmod +x $(MACOS_ARM64_DIR)/install.sh
	tar -C $(DIST_DIR) -czf $(MACOS_ARM64_ARCHIVE) $(APP_NAME)_darwin_arm64
	cp README.md $(MACOS_AMD64_DIR)/README.md
	cp scripts/install.sh $(MACOS_AMD64_DIR)/install.sh
	chmod +x $(MACOS_AMD64_DIR)/install.sh
	tar -C $(DIST_DIR) -czf $(MACOS_AMD64_ARCHIVE) $(APP_NAME)_darwin_amd64
	cp README.md $(LINUX_AMD64_DIR)/README.md
	cp scripts/install.sh $(LINUX_AMD64_DIR)/install.sh
	chmod +x $(LINUX_AMD64_DIR)/install.sh
	tar -C $(DIST_DIR) -czf $(LINUX_AMD64_ARCHIVE) $(APP_NAME)_linux_amd64
	cp README.md $(LINUX_ARM64_DIR)/README.md
	cp scripts/install.sh $(LINUX_ARM64_DIR)/install.sh
	chmod +x $(LINUX_ARM64_DIR)/install.sh
	tar -C $(DIST_DIR) -czf $(LINUX_ARM64_ARCHIVE) $(APP_NAME)_linux_arm64
	cp README.md $(WINDOWS_AMD64_DIR)/README.md
	cd $(DIST_DIR) && zip -qr $(APP_NAME)_windows_amd64.zip $(APP_NAME)_windows_amd64

install: build
	./scripts/install.sh $(BIN_DIR)/$(APP_NAME)

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
