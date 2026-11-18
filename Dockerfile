# Multi-stage сборка execai (CLI-агент). Артефакты под три цели лежат в /out
# и забираются Jenkins-пайплайном через docker cp. Сам образ запускает
# linux/amd64 бинарь как ENTRYPOINT — на случай docker run velesbsdllc/agent-vbai.
FROM golang:1.24-bookworm AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends zip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

RUN set -eux; \
    mkdir -p /out /tmp/build; \
    GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-linux-amd64        ./cmd/execai; \
    GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-linux-arm64        ./cmd/execai; \
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-windows-amd64.exe  ./cmd/execai; \
    GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-windows-arm64.exe  ./cmd/execai; \
    GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-darwin-amd64       ./cmd/execai; \
    GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tmp/build/execai-darwin-arm64       ./cmd/execai; \
    cd /tmp/build; \
    tar -czf /out/execai-linux-amd64.tar.gz    execai-linux-amd64; \
    tar -czf /out/execai-linux-arm64.tar.gz    execai-linux-arm64; \
    zip      /out/execai-windows-amd64.zip     execai-windows-amd64.exe; \
    zip      /out/execai-windows-arm64.zip     execai-windows-arm64.exe; \
    tar -czf /out/execai-darwin-amd64.tar.gz   execai-darwin-amd64; \
    tar -czf /out/execai-darwin-arm64.tar.gz   execai-darwin-arm64; \
    cd /out; \
    sha256sum execai-linux-amd64.tar.gz execai-linux-arm64.tar.gz execai-windows-amd64.zip execai-windows-arm64.zip execai-darwin-amd64.tar.gz execai-darwin-arm64.tar.gz > SHA256SUMS; \
    cp /src/scripts/install.sh /src/scripts/install.ps1 /out/; \
    printf '%s' "${VERSION}" > /out/VERSION.txt

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out /out
COPY --from=builder /tmp/build/execai-linux-amd64 /usr/local/bin/execai
RUN chmod 0755 /usr/local/bin/execai
ENTRYPOINT ["/usr/local/bin/execai"]
