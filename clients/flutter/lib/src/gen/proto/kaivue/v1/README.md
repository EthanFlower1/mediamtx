## Generated Dart protobuf stubs

This directory contains Dart protobuf message classes generated from
`internal/shared/proto/kaivue/v1/*.proto`.

**Do not edit these files by hand.** Regenerate with:

```sh
./scripts/buf-gen-dart.sh
```

Prerequisites: `buf` CLI + `protoc-gen-dart` (see script header for install).

The hand-written placeholder files in this directory (`*_pb.dart`) mirror the
proto schema and will be replaced by the real generated output once the CI
pipeline installs the Dart protoc plugin. They exist so the adapter layer in
`lib/proto_adapters/` can compile and be tested today.
