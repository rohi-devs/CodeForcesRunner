# Makefile for multi_lang_runner
APP_NAME := cfr
BUILD_DIR := build
INSTALL_DIR := /usr/local/bin
SRC := main.go

# Detect OS for optional adjustments
OS := $(shell uname -s)

.PHONY: all build install uninstall clean run

all: build

build:
	@echo "🔧 Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(APP_NAME) $(SRC)
	@echo "✅ Build complete: $(BUILD_DIR)/$(APP_NAME)"

install: build
	@echo "🚀 Installing $(APP_NAME) to $(INSTALL_DIR)..."
	@sudo cp $(BUILD_DIR)/$(APP_NAME) $(INSTALL_DIR)/
	@sudo chmod +x $(INSTALL_DIR)/$(APP_NAME)
	@echo "✅ Installed successfully. You can now run '$(APP_NAME)' from anywhere."

uninstall:
	@echo "🗑️  Removing $(APP_NAME) from $(INSTALL_DIR)..."
	@sudo rm -f $(INSTALL_DIR)/$(APP_NAME)
	@echo "✅ Uninstalled $(APP_NAME)."

clean:
	@echo "🧹 Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@echo "✅ Clean complete."

