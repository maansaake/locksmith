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

govulncheck:
	go tool -modfile tools/go.mod govulncheck ./...

unit-test:
	go test ./... -failfast

integration-test:
	go test ./test/integration/...

run: build
	./build/locksmith

run-docker: build-image
	docker run \
		-p 8080:8080 \
		github.com/maansaake/locksmith:local
