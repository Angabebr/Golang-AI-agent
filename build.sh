#!/bin/bash

echo "Building Golang AI Agent..."
echo

# Создаем директорию для билдов
mkdir -p bin

# Собираем для текущей платформы
echo "Building..."
go build -o bin/golang-ai-agent -ldflags="-s -w" .

if [ $? -eq 0 ]; then
    echo
    echo "✅ Build successful!"
    echo "Executable: bin/golang-ai-agent"
    echo
    echo "To run: ./bin/golang-ai-agent"
else
    echo
    echo "❌ Build failed!"
    exit 1
fi

