.PHONY: build
build:
	mkdir -p build
	go build -o build/locksmith

build-image:
	docker build -t github.com/maansaake/locksmith:local .

buildctl:
	mkdir -p build
	go build -o build/locksmithctl ./cmd/locksmithctl

lint:
	golangci-lint run --fix

unit-test:
	go test ./... -failfast

govulncheck:
	go tool -modfile tools/go.mod govulncheck ./...

run: build
	./build/locksmith
