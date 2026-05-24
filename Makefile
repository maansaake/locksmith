LOCKSMITH_HOST ?= localhost
LOCKSMITH_PORT ?= 9000
LOCKSMITH_METRICS_PORT ?= 9464
LOCKSMITH_OBSERVABILITY ?= true
LOAD_TEST_DURATION ?= 60s

CTL_BIN_NAME ?= locksmithctl
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin

.PHONY: build
build:
	mkdir -p build
	go build -o build/locksmith

compose/down:
	docker compose \
	-f test/compose/compose.yaml \
	down

compose/logs:
	docker compose -f test/compose/compose.yaml logs

compose/logs-follow:
	docker compose -f test/compose/compose.yaml logs -f

compose/up:
	LOCKSMITH_PORT=${LOCKSMITH_PORT} \
	docker compose \
	-f test/compose/compose.yaml \
	up \
	-d

coverage:
	@go tool cover -html=build/coverage.out -o build/coverage.html
	@go tool cover -func=build/coverage.out | awk 'END {print $$3}'

coverage/report:
	@go tool cover -html=build/coverage.out -o build/coverage.html
	@echo "### Code Coverage: $$(go tool cover -func=build/coverage.out | awk '/^total:/{print $$3}')"

ctl/build:
	mkdir -p build
	go build -o build/locksmithctl ./cmd/locksmithctl

ctl/build-release:
	mkdir -p build
	go build -trimpath -ldflags="-s -w" -o build/${CTL_BIN_NAME} ./cmd/locksmithctl

ctl/run: ctl/build
	./build/${CTL_BIN_NAME}

docker/build:
	docker build -t ghcr.io/maansaake/locksmith:local .

docker/logs:
	@docker logs locksmith

docker/run: docker/build
	docker run -d \
	-p ${LOCKSMITH_METRICS_PORT}:${LOCKSMITH_METRICS_PORT} \
	-p ${LOCKSMITH_PORT}:${LOCKSMITH_PORT} \
	-e LOCKSMITH_PORT=${LOCKSMITH_PORT} \
	-e LOCKSMITH_OBSERVABILITY=${LOCKSMITH_OBSERVABILITY} \
	-e OTEL_METRICS_EXPORTER=prometheus \
	-e OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0 \
	-e OTEL_EXPORTER_PROMETHEUS_PORT=${LOCKSMITH_METRICS_PORT} \
	--rm \
	--name locksmith \
ghcr.io/maansaake/locksmith:local

docker/stop:
	@docker stop locksmith || true

install/lint:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOBIN) v2.12.2

run: build
	LOCKSMITH_PORT=${LOCKSMITH_PORT} \
	LOCKSMITH_OBSERVABILITY=${LOCKSMITH_OBSERVABILITY} \
	OTEL_METRICS_EXPORTER=prometheus \
	    OTEL_EXPORTER_PROMETHEUS_HOST=localhost \
	    OTEL_EXPORTER_PROMETHEUS_PORT=${LOCKSMITH_METRICS_PORT} \
	./build/locksmith

static-analysis/lint:
	golangci-lint run --fix

static-analysis/vulncheck:
	go tool -modfile tools/go.mod govulncheck ./...

static-analysis/vulncheck-sarif:
	mkdir -p build
	go tool -modfile tools/go.mod govulncheck -format sarif ./... > build/govulncheck-report.sarif

test/integration:
	go test ./test/integration/... -failfast -count=1 -v

test/integration-json:
	mkdir -p build
	go test ./test/integration/... -failfast -count=1 -v -json > build/integration-test-output.json

test/load:
	rm -rf build/load-test
	mkdir -p build/load-test
	go run ./test/load cli \
	--duration ${LOAD_TEST_DURATION} \
	--report-path build/load-test/report.yaml \
	--locksmith-load.host ${LOCKSMITH_HOST} \
	--locksmith-load.port ${LOCKSMITH_PORT}
	go run ./test/load/validate \
	-error-log build/load-test/error.log \
	-report build/load-test/report.yaml

test/unit:
	go test ./pkg/... ./internal/... -failfast

test/unit-cover:
	go test ./pkg/... ./internal/... -failfast -coverprofile=build/coverage.out

test/unit-json:
	mkdir -p build
	go test ./pkg/... ./internal/... -failfast -coverprofile=build/coverage.out -v -json > build/unit-test-output.json
