# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS BUILD

ENV CGO_ENABLED 0
ENV GOPATH /go
ENV GOCACHE /go-build

RUN apk add build-base alpine-sdk

WORKDIR /app

# RUN apk update && apk add --no-cache musl-dev gcc build-base

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod/cache \
    go mod download && go mod verify

# copy the rest of the files
COPY . .

# build the binary
RUN --mount=type=cache,target=/go/pkg/mod/cache \
    --mount=type=cache,target=/go-build \
    go build -a -installsuffix cgo -v -o /app/bot cmd/bot/entry.go

FROM alpine as deployment

WORKDIR /app

COPY --from=BUILD /app/bot /app/bot

ENTRYPOINT ["/app/bot"]