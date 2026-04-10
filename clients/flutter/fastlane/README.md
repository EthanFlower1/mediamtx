# Fastlane — Per-integrator mobile build lanes (KAI-354)

This repo ships one Flutter codebase and builds N branded mobile apps from it,
one per integrator. Fastlane is scoped per-platform:

- `clients/flutter/ios/fastlane/Fastfile` — lane `ios_integrator_release`
- `clients/flutter/android/fastlane/Fastfile` — lane `android_integrator_release`

Both lanes are invoked by `.github/workflows/integrator-mobile-build.yml`
after `flutter build ipa` / `flutter build appbundle` has produced the
unsigned binary. They do NOT build the Flutter app themselves.

## Parallelism model

```
.github/workflows/integrator-mobile-build-all.yml
  -> discover whitelabel/integrators/*.json
  -> matrix fan-out, max-parallel=10
      -> .github/workflows/integrator-mobile-build.yml  (per integrator)
           -> build-ios      (macos-14)
           -> build-android  (ubuntu-24.04)
           -> notify-status  (webhook)
```

One integrator failing never cancels the others (`fail-fast: false`).

## Required environment variables

Both lanes read per-integrator inputs from env. The GitHub Actions workflow
assembles these from the manifest (`whitelabel/integrators/<id>.json`) plus
the credential vault (KAI-355, not yet implemented).

### Common (from manifest)

| Var | Example | Source |
|---|---|---|
| `INTEGRATOR_ID` | `acme` | manifest `id` |
| `BUNDLE_ID` | `com.acme.kaivue` | manifest `bundleId` |
| `APP_NAME` | `Acme Security` | manifest `appName` |
| `VERSION_NAME` | `1.2.3` | manifest `version.name` |
| `VERSION_CODE` | `42` | manifest `version.code` |

### iOS (from KAI-355 vault)

| Var | Purpose |
|---|---|
| `MATCH_PASSWORD` | Fastlane match encryption passphrase |
| `MATCH_GIT_BASIC_AUTHORIZATION` | `base64(user:token)` for the match git repo |
| `APP_STORE_CONNECT_API_KEY_ID` | App Store Connect API key id |
| `APP_STORE_CONNECT_API_KEY_ISSUER_ID` | ASC issuer id |
| `APP_STORE_CONNECT_API_KEY_KEY` | `.p8` key contents (literal) |
| `APPLE_TEAM_ID` | (optional) Apple developer team id |

### Android (from KAI-355 vault)

| Var | Purpose |
|---|---|
| `ANDROID_KEYSTORE_BASE64` | `base64(keystore.jks)` |
| `ANDROID_KEYSTORE_PASSWORD` | Keystore password |
| `ANDROID_KEY_ALIAS` | Signing key alias |
| `ANDROID_KEY_PASSWORD` | Signing key password |
| `SUPPLY_JSON_KEY_DATA` | Play Service Account JSON (literal) |

## Local testing

```bash
cd clients/flutter/ios
bundle install
INTEGRATOR_ID=acme BUNDLE_ID=com.acme.kaivue \
  bundle exec fastlane ios ios_integrator_release
```

(Will fail without real credentials — that's KAI-355's job.)

## Blocking TODOs

- [ ] KAI-353 — brand config loader writes Info.plist / AndroidManifest /
      assets / strings from the manifest. Currently only bundle id + display
      name are stubbed.
- [ ] KAI-355 — per-integrator credential vault resolves every secret above
      by `INTEGRATOR_ID`. Until then every run fails at the match/supply step.
- [ ] Flutter `integrator` flavor must be added to `ios/Runner.xcodeproj` and
      `android/app/build.gradle.kts` — see design doc.
