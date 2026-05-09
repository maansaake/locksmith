LOCKSMITH_PORT ?= 9000
LOCKSMITH_METRICS_PORT ?= 9464
LOCKSMITH_OBSERVABILITY ?= true

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
	go test ./pkg/... -failfast
	go test ./internal/... -failfast

integration-test:
	go test ./test/integration/... -failfast -count=1 -v

integration-test-json:
	mkdir -p build
	go test ./test/integration/... -failfast -count=1 -v -json > build/integration-test-output.json

compose:
	LOCKSMITH_PORT=${LOCKSMITH_PORT} \
		docker compose \
		-f test/compose/compose.yaml \
		up \
		-d

compose-down:
	docker compose \
		-f test/compose/compose.yaml \
		down

compose-logs:
	docker compose -f test/compose/compose.yaml logs

compose-logs-f:
	docker compose -f test/compose/compose.yaml logs -f

run: build
	LOCKSMITH_PORT=${LOCKSMITH_PORT} \
		LOCKSMITH_OBSERVABILITY=${LOCKSMITH_OBSERVABILITY} \
		OTEL_METRICS_EXPORTER=prometheus \
    OTEL_EXPORTER_PROMETHEUS_HOST=localhost \
    OTEL_EXPORTER_PROMETHEUS_PORT=${LOCKSMITH_METRICS_PORT} \
		./build/locksmith

run-docker: build-image
	docker run -d \
		-p ${LOCKSMITH_PORT}:${LOCKSMITH_PORT} \
		-e LOCKSMITH_PORT=${LOCKSMITH_PORT} \
		--name locksmith \
		github.com/maansaake/locksmith:local
