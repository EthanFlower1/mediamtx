#!/usr/bin/env bash
# buf-gen-dart.sh — Generate Dart protobuf stubs from kaivue v1 protos.
#
# Run from the repo root:
#   ./scripts/buf-gen-dart.sh
#
# Prerequisites:
#   1. buf CLI: https://buf.build/docs/installation
#   2. protoc-gen-dart: dart pub global activate protoc_plugin
#      (ensure ~/.pub-cache/bin is on PATH)
#
# Output lands at clients/flutter/lib/src/gen/proto/kaivue/v1/*.pb.dart

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PROTO_DIR="$REPO_ROOT/internal/shared/proto"
OUT_DIR="$REPO_ROOT/clients/flutter/lib/src/gen/proto"

# Verify prerequisites
if ! command -v buf &>/dev/null; then
  echo "ERROR: buf CLI not found. Install from https://buf.build/docs/installation" >&2
  exit 1
fi

if ! command -v protoc-gen-dart &>/dev/null; then
  echo "ERROR: protoc-gen-dart not found. Run: dart pub global activate protoc_plugin" >&2
  exit 1
fi

# Clean previous output
rm -rf "$OUT_DIR/kaivue"

# Generate
echo "Generating Dart protobuf stubs..."
cd "$PROTO_DIR"
buf generate --template buf.gen.dart.yaml .

# Count generated files
COUNT=$(find "$OUT_DIR" -name '*.pb.dart' 2>/dev/null | wc -l | tr -d ' ')
echo "Generated $COUNT .pb.dart files in $OUT_DIR/kaivue/v1/"
