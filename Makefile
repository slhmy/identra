GO ?= go
BUF ?= buf
GOLANGCI_LINT_VERSION ?= v2.12.0

LOCAL_BIN := $(CURDIR)/bin
PATH_WITH_TOOLS := $(LOCAL_BIN):$(PATH)
GOLANGCI_LINT := $(LOCAL_BIN)/golangci-lint

PROTO_TOOLS := \
	google.golang.org/protobuf/cmd/protoc-gen-go \
	google.golang.org/grpc/cmd/protoc-gen-go-grpc \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
	github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2

.PHONY: verify test vet lint proto-lint arch-check generate generate-check tools proto-tools clean-tools

verify: vet test lint arch-check generate-check

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint: $(GOLANGCI_LINT) proto-lint
	$(GOLANGCI_LINT) run

proto-lint:
	$(BUF) lint

arch-check:
	! rg -n "github.com/slhmy/identra/internal/(app|bootstrap|cache|config|gateway|mail|oauth|store)" internal/identra

generate: proto-tools
	PATH="$(PATH_WITH_TOOLS)" $(BUF) generate --clean

generate-check: generate
	git diff --exit-code gen

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
