# syntax=docker/dockerfile:1
FROM golang:1.26.3@sha256:6df14f4a4bc9d979a3721f488981e0d1b318006377e473ed23d026796f5f4c0a AS builder

WORKDIR /

FROM builder AS deps

COPY go.mod go.sum ./

RUN go mod download

FROM deps AS build

COPY . .

ENV GOOS=linux
ENV CGO_ENABLED=0

RUN --mount=type=cache,target="/root/.cache/go-build" \
  go build -trimpath -ldflags="-s -w" -o locksmith .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639 AS runtime

COPY --from=build /locksmith /locksmith

ARG VERSION
ARG COMMIT
ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}

EXPOSE 9000

ENTRYPOINT [ "/locksmith" ]
