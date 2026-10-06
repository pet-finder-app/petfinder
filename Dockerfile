# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/petfinder ./cmd/api

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S petfinder \
    && adduser -S -G petfinder petfinder

COPY --from=build /out/petfinder /usr/local/bin/petfinder

USER petfinder
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/petfinder"]
