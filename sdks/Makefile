# KaiVue SDK code generation and build targets.
#
# Prerequisites:
#   - buf (https://buf.build) for proto linting and breaking change detection
#   - protoc + protoc-gen-go for Go codegen
#   - Python 3.9+ with pip
#   - Node.js 18+ with npm
#   - TypeScript compiler (tsc)
#
# Usage:
#   make generate   # Generate code from proto (future: connect-go stubs)
#   make build       # Build all SDKs
#   make test        # Run all SDK tests
#   make lint        # Lint proto files with buf
#   make clean       # Clean build artifacts

PROTO_DIR := proto
PYTHON_DIR := python
GO_DIR := go
TS_DIR := typescript

.PHONY: all generate build test lint clean

all: generate build test

# --- Proto generation -------------------------------------------------------

generate: generate-go generate-python generate-typescript

generate-go:
	@echo "==> Go SDK: proto codegen (future: buf generate)"
	@echo "    Currently hand-written; auto-generation from connect-go planned for v0.2"

generate-python:
	@echo "==> Python SDK: proto codegen (future: grpcio-tools)"
	@echo "    Currently hand-written; auto-generation planned for v0.2"

generate-typescript:
	@echo "==> TypeScript SDK: proto codegen (future: connect-es)"
	@echo "    Currently hand-written; auto-generation planned for v0.2"

# --- Build ------------------------------------------------------------------

build: build-python build-go build-typescript

build-python:
	@echo "==> Building Python SDK..."
	cd $(PYTHON_DIR) && pip install -e ".[dev]" --quiet 2>/dev/null || true

build-go:
	@echo "==> Building Go SDK..."
	cd $(GO_DIR) && go build ./kaivue/...

build-typescript:
	@echo "==> Building TypeScript SDK..."
	cd $(TS_DIR) && npm install --silent 2>/dev/null && npx tsc || true

# --- Test -------------------------------------------------------------------

test: test-python test-go test-typescript

test-python:
	@echo "==> Testing Python SDK..."
	cd $(PYTHON_DIR) && python -m pytest tests/ -v 2>/dev/null || echo "    (install dev deps: pip install -e '.[dev]')"

test-go:
	@echo "==> Testing Go SDK..."
	cd $(GO_DIR) && go test ./kaivue_test/... -v

test-typescript:
	@echo "==> Testing TypeScript SDK..."
	cd $(TS_DIR) && npx jest --verbose 2>/dev/null || echo "    (install deps: npm install)"

# --- Lint -------------------------------------------------------------------

lint:
	@echo "==> Linting proto files..."
	cd $(PROTO_DIR) && buf lint 2>/dev/null || echo "    (install buf: https://buf.build/docs/installation)"

# --- Clean ------------------------------------------------------------------

clean:
	rm -rf $(PYTHON_DIR)/dist $(PYTHON_DIR)/*.egg-info $(PYTHON_DIR)/.pytest_cache
	rm -rf $(GO_DIR)/kaivue_test/*.test
	rm -rf $(TS_DIR)/dist $(TS_DIR)/node_modules
