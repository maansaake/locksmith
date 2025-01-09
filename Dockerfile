# syntax=docker/dockerfile:1
FROM golang:1.23 AS build

WORKDIR /

COPY . .
RUN go mod download

ARG VERSION
ARG COMMIT
ENV VERSION=${VERSION}
ENV COMMIT=${COMMIT}

RUN CGO_ENABLED=0 GOOS=linux ./build-set-version locksmith ./cmd/locksmith

FROM alpine:latest

WORKDIR /

COPY --from=build /locksmith .

EXPOSE 9000
CMD ["./locksmith"]
