# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.24.4

# Build on the target platform so CGO (sqlite) works for multi-arch builds.
FROM --platform=$TARGETPLATFORM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
  gcc \
  libc6-dev \
  && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# SERVICE must match a folder under ./cmd.
ARG SERVICE=identra-grpc
ARG CGO_ENABLED=1

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=$CGO_ENABLED \
  go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

RUN mkdir -p /out/data

# Runtime image (includes glibc needed for CGO builds like go-sqlite3)
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app

# Run as nonroot user
COPY --chown=65532:65532 --from=build /out/data /app/data
COPY --chown=65532:65532 --from=build /out/app /app/app
COPY --chown=65532:65532 --from=build /src/config.toml /app/config.toml

USER nonroot:nonroot
ENTRYPOINT ["/app/app"]
