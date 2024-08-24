# Locksmith <!-- omit in toc -->

![build](https://github.com/maansthoernvik/locksmith/actions/workflows/build.yml/badge.svg)
[![codecov](https://codecov.io/gh/maansthoernvik/locksmith/graph/badge.svg?token=6MrGbVWC5b)](https://codecov.io/gh/maansthoernvik/locksmith)
![tag](https://img.shields.io/github/v/tag/maansthoernvik/locksmith?label=latest%20version)

- [Install](#install)
- [How to run](#how-to-run)
  - [The locksmith server](#the-locksmith-server)
    - [Locksmith server environment variables](#locksmith-server-environment-variables)
    - [Advanced configuration options](#advanced-configuration-options)
  - [The command line utility](#the-command-line-utility)
- [How to use the locksmith code as a library](#how-to-use-the-locksmith-code-as-a-library)
- [Metrics](#metrics)


Locksmith provides a simple way to obtain shared locks between applications.

This project provides both server software, a command line utility, and a sample client. The protocol package can also be used to write custom client software.

## Install

The locksmith server can be installed in two different ways. To get the server container image, run:

```bash
docker pull ghcr.io/maansthoernvik/locksmith:latest
```
*You can browse available versions here: https://github.com/maansthoernvik/locksmith/pkgs/container/locksmith*

Run `go install` to instead get the server binary, make sure you have set either `GOPATH` or `GOBIN`.

```bash
go install github.com/maansthoernvik/locksmith/cmd/locksmith@v0.1.0
```

The command line utility must be installed via `go install`.

```bash
go install github.com/maansthoernvik/locksmith/cmd/locksmithctl@v0.1.0
```

## How to run

### The locksmith server

Either...
```bash
docker run -e LOCKSMITH_LOG_LEVEL=INFO ghcr.io/maansthoernvik/locksmith:latest
```

Or...
```bash
LOCKSMITH_LOG_LEVEL=INFO locksmith
```

Both the binary and the docker container have configuration options that are consumed via environment variables. All variables are namespaced and prefixed with `LOCKSMITH_` to avoid collisions.

#### Locksmith server environment variables

- `LOCKSMITH_LOG_LEVEL`: If set, the given value MUST be either `DEBUG`, `INFO`, `WARNING`, `ERROR`, or `CRITICAL` (default: `WARNING`)
- `LOCKSMITH_LOG_OUTPUT`: Set to `stdout` to log to STDOUT instead of STDERR (default: stderr)
- `LOCKSMITH_LOG_OUTPUT_CONSOLE`: Set to `true` to disable JSON logging (default: false)
- `LOCKSMITH_PORT`: The port where the locksmith server is reachable (default: `9000`)
- `LOCKSMITH_TLS`: If set to `true`, TLS is enabled for the locksmith server (default: `false`). When enabled, both `LOCKSMITH_TLS_CERT_PATH` and `LOCKSMITH_TLS_KEY_PATH` must be provided or locksmith will panic
- `LOCKSMITH_TLS_CERT_PATH`: Absolute path to the server´s certificate
- `LOCKSMITH_TLS_KEY_PATH`: Absolute path to the server´s private key
- `LOCKSMITH_TLS_REQUIRE_CLIENT_CERT`: When set to `true` (default: `false`), client connections will have their certificates validated against the client CA certificate. You must provide `LOCKSMITH_TLS_CLIENT_CA_CERT_PATH` when this variable is set
- `LOCKSMITH_TLS_CLIENT_CA_CERT_PATH`: Absolute path to the client CA certificate
- `LOCKSMITH_METRICS`: set to `true` to enable exposure of Prometheus metrics (default: `false`)

#### Advanced configuration options

These options are available to enable optimizations for systems where locksmith is expected to handle very high loads. These parameters are not recommended to be changed at all for most use cases as the defaults are meant to be good enough for 99% of cases. Ensure that any changes to these values are preceded by rigorous load testing in the intended environment.

- `LOCKSMITH_Q_TYPE`: Testing utility, there are only two options: `single` and `multi` (default: `multi`). The `single` option completely removes concurrency, making locksmith inefficient at handling higher throughput since it congests lock access to one go-routine, but making it easier to test in some situations
- `LOCKSMITH_Q_CONCURRENCY`: Only applicable for `multi` type queueing, sets the number of go-routines serving incoming requests (default: `10`)
- `LOCKSMITH_Q_CAPACITY`: Only applicable for `multi` type queueing, sets the size of each serving go-routines work queue (default: `100`)

### The command line utility

Start a new session:

```bash
locksmithctl
Starting Locksmith shell...
CONNECTED: localhost:9000

Session started, the following commands are supported:

acquire [lock]
release [lock]
> 
```

Acquire a lock:

```bash
> acquire 123
acquired  123
```

Acquire it again (locksmith closes the connection due to bad behavior)

```bash
> acquire 123
Timed out waiting for acquired signal
```

The command line utility is stupidly simple, and only really available to test connections.

## How to use the locksmith code as a library

Import and use the client in your own Go-code:

```golang
package main

import (
  "fmt"

  "github.com/maansthoernvik/locksmith/pkg/client"
)

func main() {
  acquiredFunc := func(lockTag string) {
    fmt.Println("acquired lock tag: " + lockTag)
  }

  locksmithClient := client.NewClient(&client.ClientOptions{
    Host: "localhost",
    Port: 9000,
    OnAcquired: acquiredFunc,
  })

  if err := locksmithClient.Connect(); err != nil {
    panic("uh oh, client failed to connect :-(")
  }

  if err := client.Acquire("some-lock-tag"); err != nil {
    panic("failed to acquire")
  }

  // await call to acquiredFunc
}
```

Or use the protocol package directly to write your own client. See the `ClientMessage` and `ServerMessage` types and the interface functions used for encoding/decoding.

## Metrics

Locksmith exposes a few simple Prometheus metrics:

 - `locksmith_total_locked_locks`: Gauge showing the number of currently locked locks.
 - `locksmith_acquires`: Counter showing the total numnber of (successful) acquires since start
 - `locksmith_releases`: Counter showing the total number of (successful) releases since start
 - `locksmith_rejections`: Counter vector showing the number of rejections due to client misbehavior. Vector labels are: `bad_manners`, `unnecessary_acquire`, and `unnecessary_release`

In addition to the above, locksmith also exposes all metrics provided by the `promhttp` package, providing insight into Golang performance.

To enable metrics, remember to set the environment variable `LOCKSMITH_METRICS` to `true`.
