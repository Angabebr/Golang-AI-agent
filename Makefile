.PHONY: build test clean run

# Сборка для Windows
build:
	@echo Building Golang AI Agent...
	@if not exist bin mkdir bin
	@go build -o bin\golang-ai-agent.exe -ldflags="-s -w" .
	@echo Build complete: bin\golang-ai-agent.exe

# Сборка для Linux/Mac
build-linux:
	@echo Building Golang AI Agent for Linux...
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 go build -o bin/golang-ai-agent -ldflags="-s -w" .
	@echo Build complete: bin/golang-ai-agent

# Запуск тестов
test:
	@echo Running tests...
	@go test ./... -v

# Запуск тестов с покрытием
test-coverage:
	@echo Running tests with coverage...
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo Coverage report: coverage.html

# Очистка
clean:
	@echo Cleaning...
	@if exist bin rmdir /s /q bin
	@if exist coverage.out del coverage.out
	@if exist coverage.html del coverage.html
	@echo Clean complete

# Запуск приложения
run:
	@go run main.go

