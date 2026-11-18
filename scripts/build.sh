#!/bin/sh
# Cross-compiles everything into dist/ (works on Linux, macOS or Windows+bash).
set -e
cd "$(dirname "$0")/.."
VERSION="${VERSION:-dev}"
mkdir -p dist
VX="-X github.com/Cmacle/TheHouseAlwaysWins/internal/server.Version=$VERSION -X github.com/Cmacle/TheHouseAlwaysWins/internal/client.Version=$VERSION"
LD="-s -w $VX"
# Windows builds keep their symbols and get an icon, version info and a
# manifest: stripped, anonymous exes are what antivirus heuristics flag.
WV="$VERSION"; case "$WV" in [0-9]*) ;; *) WV=0.0.0 ;; esac
go run ./tools/winres -version "$WV" -name "The House Always Wins" -exe TheHouseAlwaysWins.exe -out cmd/client/rsrc_windows_amd64.syso
go run ./tools/winres -version "$WV" -name "The House Always Wins Server" -exe HouseServer.exe -out cmd/server/rsrc_windows_amd64.syso
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$VX -H windowsgui" -o dist/TheHouseAlwaysWins.exe ./cmd/client
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LD" -o dist/TheHouseAlwaysWins-linux-amd64 ./cmd/client
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LD" -o dist/TheHouseAlwaysWins-linux-arm64 ./cmd/client
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$VX" -o dist/HouseServer.exe ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LD" -o dist/house-server-linux-amd64 ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LD" -o dist/house-server-linux-arm64 ./cmd/server
go run ./cmd/server -print-config > dist/server.json
cp scripts/run-server.bat README.md deploy/thehousealwayswins.service dist/
echo "built into dist/"
