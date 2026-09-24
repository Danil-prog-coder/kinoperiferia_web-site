.PHONY: run test lint build

run:
	go run .

test:
	go test ./...

lint:
	gofmt -l . && go vet ./...

build:
	go build -trimpath -o kinoperiferia .
