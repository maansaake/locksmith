build:
	mkdir -p build
	go build -o build/locksmith

buildctl:
	mkdir -p build
	go build -o build/locksmithctl ./cmd/locksmithctl

lint:
	golangci-lint run --fix
