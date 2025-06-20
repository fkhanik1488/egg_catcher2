# Makefile для проекта egg_catcher2

# Переменные
PROJECT_NAME = egg_catcher2
MAIN_FILE = main.go
BINARY_DIR = bin
ASSETS_DIR = assets
OUTPUT_ZIP = $(PROJECT_NAME)_release.zip
GO = go

# Определение флагов сборки
BUILD_FLAGS = -ldflags="-s -w"

# Цель по умолчанию
all: build

# Сборка исполняемого файла
build:
	@echo "Building $(PROJECT_NAME)..."$(GO) build -o $(BINARY_DIR)/$(PROJECT_NAME).exe $(BUILD_FLAGS) $(MAIN_FILE)
	@echo "Build completed. Binary is in $(BINARY_DIR)/$(PROJECT_NAME).exe"

# Упаковка релиза с ресурсами
release: build
	@echo "Creating release package..."
	if exist $(OUTPUT_ZIP) del /F /Q $(OUTPUT_ZIP)
	cd $(BINARY_DIR) && ..\7z.exe a ..$(OUTPUT_ZIP) $(PROJECT_NAME).exe ..\$(ASSETS_DIR)\* -r
	@echo "Release package created: $(OUTPUT_ZIP)"

# Установка зависимостей
install:
	@echo "Installing dependencies..."
	$(GO) mod tidy
	$(GO) get -u ./...

# Очистка сгенерированных файлов
clean:
	@echo "Cleaning up..."
	$(GO) clean
	if exist $(BINARY_DIR) rmdir /S /Q $(BINARY_DIR)
	if exist $(OUTPUT_ZIP) del /F /Q $(OUTPUT_ZIP)
	@echo "Cleanup completed"

# Кроссплатформенная сборка
cross-build:
	@echo "Building for multiple platforms..."
	$(GO) build -o $(BINARY_DIR)/$(PROJECT_NAME)_windows.exe $(BUILD_FLAGS) $(MAIN_FILE)
	GOOS=linux GOARCH=amd64 $(GO) build -o $(BINARY_DIR)/$(PROJECT_NAME)_linux $(BUILD_FLAGS) $(MAIN_FILE)
	GOOS=darwin GOARCH=amd64 $(GO) build -o $(BINARY_DIR)/$(PROJECT_NAME)_macos $(BUILD_FLAGS) $(MAIN_FILE)
	@echo "Cross-build completed. Binaries are in $(BINARY_DIR)"

# Помощь
help:
	@echo "Available targets:"
	@echo "  all         - Build the project (default)"
	@echo "  build       - Build the executable"
	@echo "  release     - Build and create a release package with assets"
	@echo "  install     - Install dependencies"
	@echo "  clean       - Clean up generated files"
	@echo "  cross-build - Build for Windows, Linux, and macOS"
	@echo "  help        - Show this help message"

.PHONY: all build release install clean cross-build help