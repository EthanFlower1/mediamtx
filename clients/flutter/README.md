# nvr_client

The MediaMTX NVR cross-platform client. A single Flutter codebase that
ships to six targets from one `lib/` tree.

## Supported targets + CI

| Target  | Runner          | Build command                           |
| ------- | --------------- | --------------------------------------- |
| Android | `ubuntu-latest` | `flutter build apk --debug`             |
| iOS     | `macos-latest`  | `flutter build ios --debug --no-codesign` |
| macOS   | `macos-latest`  | `flutter build macos --debug`           |
| Windows | `windows-latest`| `flutter build windows --debug`         |
| Linux   | `ubuntu-latest` | `flutter build linux --debug`           |
| Web     | `ubuntu-latest` | `flutter build web`                     |

The six-target matrix is enforced in CI by
[`.github/workflows/flutter-build-matrix.yml`](../../.github/workflows/flutter-build-matrix.yml).
Every PR that touches `clients/flutter/**` runs `flutter analyze`,
`flutter test`, and a debug build on all six runners in parallel with
`fail-fast: false`, so a regression on one target does not mask
regressions on the others.

### Feature parity test suite

`test/parity/` holds a unit-level feature-parity suite
(`platform_matrix_test.dart` + `compile_check.dart`) that asserts the
same set of providers and public entry points resolves on every target.
This is intentionally a thin skeleton — the point is to catch accidental
`dart:io` leakage into web-bound code, or a plugin drop that breaks one
platform, at `flutter test` time on that platform's runner. Deeper
per-feature tests live alongside the feature code in `test/`.

Release signing, fastlane lanes, store upload, and code-signed artifact
promotion are intentionally out of scope — the matrix produces debug
builds only. Those live in follow-up tickets.

## Getting started

```sh
cd clients/flutter
flutter pub get
flutter run          # current host target
flutter test         # unit + parity suite
flutter analyze
```

See the [Flutter documentation](https://docs.flutter.dev/) for target
toolchain setup (Xcode for iOS/macOS, Android Studio for Android,
desktop toolchains for Linux/Windows).
