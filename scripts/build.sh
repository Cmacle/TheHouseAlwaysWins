#!/bin/sh
# Cross-compiles everything into dist/ (works on Linux, macOS or Windows+bash).
set -e
cd "$(dirname "$0")/.."
VERSION="${VERSION:-dev}"
mkdir -p dist
LD="-s -w -X github.com/Cmacle/TheHouseAlwaysWins/internal/server.Version=$VERSION -X github.com/Cmacle/TheHouseAlwaysWins/internal/client.Version=$VERSION"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LD -H windowsgui" -o dist/TheHouseAlwaysWins.exe ./cmd/client
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LD" -o dist/TheHouseAlwaysWins-linux-amd64 ./cmd/client
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LD" -o dist/TheHouseAlwaysWins-linux-arm64 ./cmd/client
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LD" -o dist/HouseServer.exe ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LD" -o dist/house-server-linux-amd64 ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LD" -o dist/house-server-linux-arm64 ./cmd/server
go run ./cmd/server -print-config > dist/server.json
cp scripts/run-server.bat README.md deploy/thehousealwayswins.service dist/
echo "built into dist/"
