build:
	mkdir -p build
	go build -o build/locksmith

buildctl:
	mkdir -p build
	go build -o build/locksmithctl ./cmd/locksmithctl

lint:
	golangci-lint run --fix

unit-test:
	go test ./... -failfast

govulncheck:
	go tool -modfile tools/go.mod govulncheck ./...
