# syntax=docker/dockerfile:1.7
ARG GO_IMAGE=golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651
ARG CLIENT_BASE_IMAGE=debian:bookworm-slim@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818

FROM ${GO_IMAGE} AS builder
WORKDIR /src
COPY packages/cli/go.mod ./
COPY packages/cli/ ./
ARG VERSION=0.3.0-local
ARG COMMIT=local
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE} -X github.com/billstark001/latexmk/packages/cli/internal/client.version=${VERSION}" \
    -o /out/latexmk ./cmd/latexmk

FROM ${CLIENT_BASE_IMAGE}
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       ca-certificates git \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/latexmk /usr/local/bin/latexmk
ENV HOME=/tmp
WORKDIR /workspace
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/latexmk"]
