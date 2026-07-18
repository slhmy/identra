# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.24.4

# SQLite is pure Go, so cross-platform builds do not require a C toolchain.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
  go build -trimpath \
  -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
  -o /out/identra ./cmd/identra

RUN mkdir -p /out/data

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app

# Run as nonroot user
COPY --chown=65532:65532 --from=build /out/data /app/data
COPY --chown=65532:65532 --from=build /out/identra /app/identra
COPY --chown=65532:65532 --from=build /src/config.toml /app/config.toml

USER nonroot:nonroot
ENTRYPOINT ["/app/identra"]
CMD ["serve"]
