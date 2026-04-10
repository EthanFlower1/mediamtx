# KAI-354 — Per-integrator mobile app build pipeline (design)

**Status:** Draft — blocked on KAI-353 and KAI-355.
**Owner:** lead-sre
**Linear:** https://linear.app/kaivue/issue/KAI-354
**Project:** MS: White-Label & Mobile Build Pipeline

## Why

The white-label program sells integrators a branded mobile app built on the
Kaivue Flutter codebase. Operationally this means we must be able to build,
sign, and ship N distinct iOS + Android apps from one source tree on every
green build of main, without hand-editing Xcode projects or Gradle files per
customer. This is the single most differentiated feature in the white-label
program; it has to be fully automated and reproducible.

## Dependency chain

```
KAI-353 (brand config loader)
        |
        v
KAI-354 (this doc — build pipeline)
        ^
        |
KAI-355 (per-integrator credential vault)
```

- **KAI-353** defines `BrandConfig` + the loader that stamps Info.plist,
  AndroidManifest, icon / splash / strings / colors. KAI-354 calls the loader
  as a pre-build step. Until 353 lands, the workflow only stubs `BUNDLE_ID`
  and display name (enough to prove the fan-out works).
- **KAI-355** owns credentials: Apple App Store Connect API keys, Fastlane
  match repo access, Android keystores, Play service account JSON, and the
  integrator-portal webhook URL. KAI-354 references every one of these by env
  var name only; the vault fetch step is a TODO until 355 lands.

## Architecture

```
                     push to main (clients/flutter/**)
                                  |
                                  v
          .github/workflows/integrator-mobile-build-all.yml
                                  |
          discover whitelabel/integrators/*.json  -> [acme, globex, ...]
                                  |
               matrix fan-out (fail-fast=false, max-parallel=10)
                                  |
             +--------------------+-------------------+
             |                    |                   |
             v                    v                   v
  integrator-mobile-build    integrator-mobile-build    ...
  (reusable workflow)         (reusable workflow)
             |                    |
   +---------+---------+    +---------+---------+
   |                   |    |                   |
   v                   v    v                   v
 build-ios         build-android    ...       ...
 (macos-14)        (ubuntu-24.04)
   |                   |
   +---------+---------+
             |
             v
       notify-status  (POST to INTEGRATOR_PORTAL_WEBHOOK)
```

### Why reusable workflows instead of a single giant matrix?

1. Each integrator becomes its own GitHub run with its own run URL — trivial
   to link from the integrator portal and to retry individually.
2. Concurrency group `integrator-mobile-build-<id>` serializes back-to-back
   builds of the *same* integrator (so TestFlight version codes don't race)
   while still letting different integrators build in parallel.
3. Manual rebuilds of a single integrator become `gh workflow run
   integrator-mobile-build.yml -f integrator_id=acme`, handled by
   `scripts/dispatch-integrator-builds.sh`.

### Why one job per OS (not a flavor matrix inside the job)?

- macOS runners are ~10x the cost of Linux runners; splitting lets the AAB
  land on ubuntu-24.04 and only pays for macos-14 on the iOS leg.
- Failure isolation: a flaky Xcode signing step should not block the AAB
  upload and vice versa.

## Inputs

1. Flutter source at `clients/flutter/**` (untouched by this workflow — we do
   NOT modify `lib/**` runtime code; per-integrator behavior flows through
   `--dart-define=INTEGRATOR_ID=...` at build time).
2. `whitelabel/integrators/<id>.json` — manifest (schema owned by KAI-353).
3. Per-integrator credentials from the KAI-355 vault, keyed by `INTEGRATOR_ID`.

## Outputs

- `clients/flutter/build/ios/ipa/*.ipa` — signed, ready for TestFlight
- `clients/flutter/build/app/outputs/bundle/integratorRelease/*.aab` — signed
- `clients/flutter/build/app/outputs/mapping/integratorRelease/mapping.txt`
- `clients/flutter/build/app/outputs/symbols/**` (obfuscation symbols)
- iOS dSYMs under `build/ios/archive/*.xcarchive/dSYMs/**`
- GitHub artifacts: `<id>-ios-<run_id>` and `<id>-android-<run_id>`,
  retention 30 days.
- Side effects: TestFlight build, Play internal track draft release,
  webhook POST to the integrator portal.

## Reproducibility

1. Flutter version pinned in `env.FLUTTER_VERSION` (today: `3.24.3`).
2. Ruby 3.2 + `bundler-cache: true` locks Fastlane and its deps via the
   checked-in `Gemfile.lock` (to be generated on first run).
3. `actions/checkout@v4` with `fetch-depth: 0` so `git describe` can stamp
   versions deterministically.
4. Version name/code come from the manifest, not from the build timestamp,
   so the same manifest always produces the same installed version.
5. Keystore + signing certs come from a per-integrator vault (KAI-355), not
   from random CI secrets, so rebuilds on a different runner produce
   identically-signed artifacts.

## Rebuild-on-release

`on.release: types: [published]` in `integrator-mobile-build-all.yml` fans out
every integrator when we cut a Kaivue release. This gives integrators a fresh
TestFlight / Play internal build pinned to a known Kaivue release tag without
any human running `gh workflow run`.

## Acceptance criteria mapping

| Linear criterion | Where it lives |
|---|---|
| GitHub Actions matrix, one job per integrator | `integrator-mobile-build-all.yml` fan-out |
| Inputs: Flutter + brand config + integrator creds | manifest + KAI-355 vault refs |
| Outputs: signed `.ipa` + `.aab` | `build-ios` / `build-android` jobs |
| Upload to TestFlight + Play internal | Fastlane `pilot` / `supply` lanes |
| Per-integrator bundle id/name/assets | `resolve-manifest` + KAI-353 loader |
| Manifest-driven | `whitelabel/integrators/<id>.json` |
| Reproducible | pinned versions + vault-sourced signing |
| Rebuild-on-release | `on.release: types: [published]` |
| Sample integrator round-trip | `whitelabel/integrators/example-acme.json` |

## Blocking TODO list (must clear before ready-for-review)

- [ ] **KAI-353 landed** — brand config loader wired into the
      `Apply brand manifest` step (replaces the `PlistBuddy` stub).
- [ ] **KAI-355 landed** — vault step resolves per-integrator secrets by
      `INTEGRATOR_ID`; remove every `# TODO(KAI-355)` marker.
- [ ] Flutter `integrator` flavor added to
      `clients/flutter/ios/Runner.xcodeproj` (scheme + build configurations).
- [ ] Flutter `integrator` flavor added to
      `clients/flutter/android/app/build.gradle.kts` (productFlavors block).
- [ ] `clients/flutter/ios/ExportOptions.plist` committed (method=app-store,
      provisioningProfiles resolved from match).
- [ ] `clients/flutter/ios/Gemfile.lock` and
      `clients/flutter/android/Gemfile.lock` committed after a clean
      `bundle install` on a macOS / Linux runner respectively.
- [ ] Real `whitelabel/integrators/assets/acme/*` fixtures committed (or
      moved to a separate `whitelabel-assets` repo if licensing requires it).
- [ ] End-to-end smoke: dispatch `example-acme` → TestFlight build appears →
      Play internal draft appears → webhook delivered. (Depends on 353+355.)
- [ ] Cost guardrail: confirm macos-14 minutes budget can absorb
      `len(integrators) * ~25min` per main push; add `paths-ignore` for
      doc-only changes if needed.

## Out of scope (explicit non-goals)

- Implementing the brand config loader itself (KAI-353).
- Implementing the credential vault (KAI-355).
- Rewriting `clients/flutter/lib/**` to consume `INTEGRATOR_ID`.
- Production release tracks (we only target TestFlight + Play *internal*).
- macOS / Windows / Linux desktop flavors — KAI-306 already covers those.
