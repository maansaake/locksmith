# syntax=docker/dockerfile:1
FROM golang:1.26 AS build

WORKDIR /

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go mod download

RUN --mount=type=cache,target="/root/.cache/go-build" \
  CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -extldflags=-static" -o /locksmith

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=build /locksmith .

ARG VERSION
ARG COMMIT
ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}

EXPOSE 9000

ENTRYPOINT [ "/locksmith" ]
