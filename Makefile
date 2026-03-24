.PHONY: build test fmt

build:
	go build -o ./bin/temporal-ts_net ./cmd/temporal-ts_net

test:
	go test ./...

fmt:
	go fmt ./...
