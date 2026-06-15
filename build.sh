#!/bin/sh

# Build frontend from temp_frontend
cd temp_frontend
npm i
npm run build
cd ..

echo "Backend"

# Copy built frontend to web/html for Go embedding
mkdir -p web/html
rm -fr web/html/*
cp -R temp_frontend/dist/* web/html/

# Cross-compile for Linux amd64 (Debian)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-w -s" -o kwor main.go

echo "Build complete: kwor (linux/amd64)"
