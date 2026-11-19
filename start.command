#!/bin/sh
set -eu
cd "$(dirname "$0")"
go run ./cmd/setup
printf '\nStyleShop: http://127.0.0.1:8082\nДля остановки нажми Ctrl+C.\n\n'
go run ./cmd/server
