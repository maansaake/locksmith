# Locksmith <!-- omit in toc -->

[![Main branch protection](https://github.com/maansaake/locksmith/actions/workflows/main.yaml/badge.svg)](https://github.com/maansaake/locksmith/actions/workflows/main.yaml)
[![Code scanning](https://github.com/maansaake/locksmith/actions/workflows/code-scanning.yaml/badge.svg)](https://github.com/maansaake/locksmith/actions/workflows/code-scanning.yaml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/maansaake/locksmith)](https://goreportcard.com/report/github.com/maansaake/locksmith)
![tag](https://img.shields.io/github/v/tag/maansaake/locksmith?label=latest%20version)

Locksmith provides a simple way to obtain shared locks between applications.

This project provides:

- server implementation (binary + docker image)
- a command line utility `locksmithctl`
- a sample client
- client library packages (see `/pkg`)

Documentation provided [inline](https://pkg.go.dev/github.com/maansthoernvik/locksmith).

## Install

The locksmith server can be installed in two different ways. To get the server container image, run:

```bash
docker pull ghcr.io/maansaake/locksmith:latest
```

*You can browse available versions here: <https://github.com/maansaake/locksmith/pkgs/container/locksmith>*

Run `go install` to instead get the server binary, make sure you have set either `GOPATH` or `GOBIN`.

```bash
go install github.com/maansaake/locksmith@latest
```

## How to run

### The locksmith server

Either...

```bash
docker run ghcr.io/maansaake/locksmith:latest
```

Or...

```bash
locksmith
```

Both the binary and the docker container have configuration options that are consumed via environment variables. All variables are namespaced and prefixed with `LOCKSMITH_` to avoid collisions.

#### Locksmith server environment variables

- `LOCKSMITH_LOG_VERBOSITY`: Sets the verbosity level for logging. Higher values produce more verbose output (default: `0`)
- `LOCKSMITH_LOG_OUTPUT`: Sets the log format. Must be either `json` or `console` (default: `console`)
- `LOCKSMITH_PORT`: The port where the locksmith server is reachable (default: `9000`)
- `LOCKSMITH_TLS`: If set to `true`, TLS is enabled for the locksmith server (default: `false`). When enabled, both `LOCKSMITH_TLS_CERT_PATH` and `LOCKSMITH_TLS_KEY_PATH` must be provided
- `LOCKSMITH_TLS_CERT_PATH`: Absolute path to the server's certificate (default: `/etc/cert/locksmith.pem`)
- `LOCKSMITH_TLS_KEY_PATH`: Absolute path to the server's private key (default: `/etc/cert/locksmith.key`)
- `LOCKSMITH_TLS_REQUIRE_CLIENT_CERT`: When set to `true` (default: `false`), client connections will have their certificates validated against the client CA certificate. You must provide `LOCKSMITH_TLS_CLIENT_CA_CERT_PATH` when this variable is set
- `LOCKSMITH_TLS_CLIENT_CA_CERT_PATH`: Absolute path to the client CA certificate (default: `/etc/cert/client_ca.cert`)
- `LOCKSMITH_OBSERVABILITY`: Set to `true` to enable OpenTelemetry instrumentation (default: `false`)
- `LOCKSMITH_RUNTIME_METRICS`: Set to `false` to disable Go runtime metrics. Only applicable when `LOCKSMITH_OBSERVABILITY` is `true` (default: `true`)

#### Advanced configuration options

These options are available to enable optimizations for systems where locksmith is expected to handle very high loads. These parameters are not recommended to be changed at all for most use cases as the defaults are meant to be good enough for 99% of cases. Ensure that any changes to these values are preceded by rigorous load testing in the intended environment.

- `LOCKSMITH_Q_TYPE`: Testing utility, there are only two options: `single` and `multi` (default: `multi`). The `single` option completely removes concurrency, making locksmith inefficient at handling higher throughput since it congests lock access to one go-routine, but making it easier to test in some situations
- `LOCKSMITH_Q_CONCURRENCY`: Only applicable for `multi` type queueing, sets the number of go-routines serving incoming requests (default: `10`)
- `LOCKSMITH_Q_CAPACITY`: Only applicable for `multi` type queueing, sets the size of each serving go-routines work queue (default: `100`)

### `locksmithctl`

The command line utility must be installed via `go install`.

```bash
go install github.com/maansaake/locksmith/cmd/locksmithctl@latest
```

User guidance is provided as part of the CLI's UX:

```text
╭──────────────────────────────────────╮
│  🔑 locksmithctl                     │
│  Interactive Locksmith Lock Manager  │
│  Connecting to localhost:9000        │
╰──────────────────────────────────────╯
✓ Connected to localhost:9000

Commands
  acquire [lock]          Acquire a lock (interactive prompt if omitted)
  release [lock]          Release a lock (picker if omitted)
  list                    Show all acquired locks
  reconnect               Reconnect to the server
  help                    Show this help
  exit / quit             Exit locksmithctl

locksmith> acquire locktag
→ Acquire request sent for locktag  (waiting for server acknowledgement…)
✓ Acquired: locktag
locksmith> list
╭──────────────────╮
│ Active Locks (1) │
│    1.  locktag   │
╰──────────────────╯
```

## How to use the locksmith code as a library

Import and use the client in your own Go-code:

```golang
package main

import (
  "fmt"

  "github.com/maansaake/locksmith/pkg/client"
)

func main() {
  acquiredFunc := func(lockTag string) {
    fmt.Println("acquired lock tag: " + lockTag)
  }

  locksmithClient := client.New(&client.Opts{
    Host:       "localhost",
    Port:       9000,
    OnAcquired: acquiredFunc,
  })

  if err := locksmithClient.Connect(); err != nil {
    panic("uh oh, client failed to connect :-(")
  }

  if err := locksmithClient.Acquire("some-lock-tag"); err != nil {
    panic("failed to acquire")
  }

  // await call to acquiredFunc
}
```

Or use the protocol package directly to write your own client. See the `ClientMessage` and `ServerMessage` types and the interface functions used for encoding/decoding.

## Metrics

Locksmith uses [OpenTelemetry](https://opentelemetry.io/) for instrumentation. When `LOCKSMITH_OBSERVABILITY` is set to `true`, the OTEL SDK is initialized and the [autoexport](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport) package is used to select the metrics exporter based on standard OTEL environment variables. By default the OTLP exporter is used; set `OTEL_METRICS_EXPORTER` to change this (e.g. `prometheus`, `console`, or `none`). See the [OTEL SDK environment variable specification](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) and the [OTLP exporter configuration reference](https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/) for the full list of supported variables.

Locksmith registers the following OTEL instruments:

| Instrument | Type | Description |
|---|---|---|
| `locksmith.locks` | UpDownCounter | Number of currently held locks |
| `locksmith.acquires` | Counter | Total successful lock acquires since start |
| `locksmith.releases` | Counter | Total successful lock releases since start |
| `locksmith.rejections` | Counter | Rejections due to client misbehavior; carries a `reason` attribute (`bad_manners`, `unnecessary_acquire`, or `unnecessary_release`) |

When `LOCKSMITH_RUNTIME_METRICS` is `true` (the default), the following Go runtime instruments from the [OTEL Go runtime semconv](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/) are also recorded:

| Instrument | Unit | Description |
|---|---|---|
| `go.memory.used` | By | Memory used by the Go runtime |
| `go.memory.limit` | By | Go runtime memory limit, if configured |
| `go.memory.allocated` | By | Memory allocated to the heap |
| `go.memory.allocations` | {allocation} | Count of heap allocations |
| `go.memory.gc.goal` | By | Heap size target at end of GC cycle |
| `go.goroutine.count` | {goroutine} | Number of live goroutines |
| `go.processor.limit` | {thread} | OS threads available for user-level Go code |
| `go.config.gogc` | % | Heap size target percentage (GOGC) |

> **Note:** the instrument names above are the OTEL API names. The exact metric names seen in your monitoring system depend on the exporter. For example, the Prometheus exporter converts dots to underscores and appends `_total` to counters (`locksmith.acquires` → `locksmith_acquires_total`), while the OTLP exporter preserves the dot notation.
