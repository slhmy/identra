GO ?= go
BUF ?= buf
SQLC ?= sqlc
GOLANGCI_LINT_VERSION ?= v2.12.0
IDENTRA_REDIS_URL ?= localhost:6379

LOCAL_BIN := $(CURDIR)/bin
PATH_WITH_TOOLS := $(LOCAL_BIN):$(PATH)
GOLANGCI_LINT := $(LOCAL_BIN)/golangci-lint

PROTO_TOOLS := \
	google.golang.org/protobuf/cmd/protoc-gen-go \
	google.golang.org/grpc/cmd/protoc-gen-go-grpc

.PHONY: dev dev-infra dev-down run bootstrap-service-account verify test test-integration vet lint proto-lint arch-check generate generate-check tools proto-tools clean-tools

verify: vet test lint arch-check generate-check

dev:
	docker compose up --build

dev-infra:
	docker compose up -d redis mailpit

dev-down:
	docker compose down

run:
	SMTP_MAILER_HOST=localhost \
	SMTP_MAILER_PORT=1025 \
	SMTP_MAILER_FROM_EMAIL=noreply@identra.local \
	SMTP_MAILER_FROM_NAME="Identra Local" \
	SMTP_MAILER_START_TLS=false \
	SMTP_MAILER_AUTH_ENABLED=false \
	$(GO) run ./cmd/identra serve

bootstrap-service-account:
	$(GO) run ./cmd/identra bootstrap service-account --name platform-admin --scope identra.admin

test:
	$(GO) test ./...

test-integration:
	IDENTRA_REDIS_URL="$(IDENTRA_REDIS_URL)" $(GO) test ./internal/cache -run 'TestRedis.*Contract' -count=1

vet:
	$(GO) vet ./...

lint: $(GOLANGCI_LINT) proto-lint
	$(GOLANGCI_LINT) run

proto-lint:
	$(BUF) lint

arch-check:
	$(GO) test ./internal/arch

generate: proto-tools
	PATH="$(PATH_WITH_TOOLS)" $(BUF) generate --clean
	$(SQLC) generate

generate-check: generate
	git diff --exit-code gen internal/store/sqlite/sqlitedb

tools: $(GOLANGCI_LINT) proto-tools

proto-tools:
	@mkdir -p "$(LOCAL_BIN)"
	@for tool in $(PROTO_TOOLS); do \
		echo "installing $$tool"; \
		GOBIN="$(LOCAL_BIN)" $(GO) install "$$tool"; \
	done

$(GOLANGCI_LINT):
	@mkdir -p "$(LOCAL_BIN)"
	GOBIN="$(LOCAL_BIN)" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean-tools:
	rm -rf "$(LOCAL_BIN)"
