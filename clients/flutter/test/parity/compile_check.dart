// KAI-306 — Parity compile-check.
//
// This file exists so that `flutter analyze` and `flutter test` on every
// target runner will fail if any of the imported library entry points
// stops compiling on that target. It is intentionally import-only — there
// is no executable test in here. The real parity assertions live in
// platform_matrix_test.dart; this file is a fast "does the public surface
// still build everywhere?" smoke check.
//
// When adding a new top-level lib/ entry point that is meant to be
// cross-platform, add its import here. Platform-gated code (e.g., files
// that directly import `dart:io`) should NOT be listed here — they need a
// conditional-import wrapper instead.

// ignore_for_file: unused_import

import 'package:nvr_client/app.dart';
import 'package:nvr_client/auth/auth_providers.dart';
import 'package:nvr_client/discovery/discovery_providers.dart';
import 'package:nvr_client/state/app_session.dart';

// Intentionally empty — the imports above are the entire contract of this
// file. Keeping `main` present (but empty) means `flutter test` will load
// it as a test file and surface any compile failure on the target runner.
void main() {}
