# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/concierge .

# alpine, not scratch: scripts triggered by webhooks/cron need /bin/sh to run.
# Deployments needing extra tools (docker CLI, curl, python3, ...) should
# build a one-line derived image: FROM ghcr.io/<user>/concierge:latest + RUN apk add ...
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/concierge /usr/local/bin/concierge

ENV PORT=8080
ENV DATA_DIR=/data
ENV TOTP_ISSUER=Concierge

VOLUME /data
EXPOSE 8080
ENTRYPOINT ["concierge"]
