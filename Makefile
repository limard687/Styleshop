.PHONY: setup run build test verify
setup:
	go run ./cmd/setup
run: setup
	go run ./cmd/server
build:
	go build -o bin/styleshop ./cmd/server
test:
	go test -race ./...
verify:
	go vet ./...
	go test -race ./...
	node --check web/static/app.js
