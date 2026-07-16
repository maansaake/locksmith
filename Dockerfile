# syntax=docker/dockerfile:1
FROM golang:1.26.5@sha256:ae5a2316d12f3e78fd99177dad452e6ad4f240af2d71d57b480c3477f250fec6 AS builder

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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:b7bb25d9f7c31d2bdd1982feb4dafcaf137703c7075dbe2febb41c24212b946f AS runtime

COPY --from=build /locksmith /locksmith

ARG VERSION
ARG COMMIT
ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}

EXPOSE 9000

ENTRYPOINT [ "/locksmith" ]
