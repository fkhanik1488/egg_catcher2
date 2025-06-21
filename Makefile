# Makefile для проекта egg_catcher2

# Переменные
PROJECT_NAME = egg_catcher2
MAIN_FILE = main.go
BINARY_DIR = bin
GO = go

# Определение флагов сборки
BUILD_FLAGS = -ldflags="-s -w"

# Цель по умолчанию
all: build

# Сборка исполняемого файла
build:
	@echo Building $(PROJECT_NAME)...
	@if not exist $(BINARY_DIR) mkdir $(BINARY_DIR)
	@$(GO) build -o $(BINARY_DIR)/$(PROJECT_NAME).exe $(BUILD_FLAGS) $(MAIN_FILE)
	@echo Build completed. Binary is in $(BINARY_DIR)/$(PROJECT_NAME).exe

# Установка зависимостей
install:
	@echo Installing dependencies...
	@$(GO) mod tidy
	@$(GO) get -u ./...

# Очистка сгенерированных файлов
clean:
	@echo Cleaning up...
	@$(GO) clean
	@if exist $(BINARY_DIR) rmdir /S /Q $(BINARY_DIR)
	@echo Cleanup completed

.PHONY: all build install clean