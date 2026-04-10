# Integrator manifests (KAI-354)

Each file in this directory is a single-integrator brand manifest consumed by
`.github/workflows/integrator-mobile-build.yml`. The filename stem must match
the `id` field.

The schema matches `internal/cloud/whitelabel/BrandConfig` (KAI-353, in
parallel drafting). Until KAI-353 lands we keep this README as the source of
truth and validate via `python3 -m json.tool`.

## Required fields

```jsonc
{
  // Stable machine identifier — used as filename, flavor suffix, CI matrix key.
  "id": "acme",

  // Marketing name shown on home screen, splash, App Store listing.
  "appName": "Acme Security",

  // iOS CFBundleIdentifier / Android applicationId — MUST be unique per integrator.
  "bundleId": "com.acme.kaivue",

  // Semantic version + monotonic build number.
  "version": {
    "name": "1.0.0",
    "code": 1
  },

  // Visual branding. Paths are relative to this manifest file.
  "branding": {
    "primaryColor":   "#0B5CD5",
    "secondaryColor": "#F2B441",
    "logo":           "assets/acme/logo.svg",
    "icon":           "assets/acme/icon-1024.png",
    "splash":         "assets/acme/splash.png"
  },

  // Localized strings injected into Info.plist + strings.xml.
  "strings": {
    "supportEmail": "support@acme.example",
    "websiteUrl":   "https://acme.example",
    "tagline":      "Professional security, simplified."
  },

  // Platform-store metadata.
  "store": {
    "appleTeamId":       "TODO_KAI_355",
    "appStoreAppId":     "0000000000",
    "playPackageName":   "com.acme.kaivue",
    "privacyPolicyUrl":  "https://acme.example/privacy"
  }
}
```

## Adding a new integrator

1. Copy `example-acme.json` to `<id>.json`.
2. Fill in the fields above; every `TODO_KAI_355` must be replaced or moved
   into the credential vault (KAI-355) — do NOT commit real secrets here.
3. Drop brand assets under `whitelabel/integrators/assets/<id>/`.
4. Open a PR. The `integrator-mobile-build-all` workflow will fan out a new
   build job automatically on merge to main.

## Validation

```bash
python3 -c "import json,sys; [json.load(open(f)) for f in sys.argv[1:]]" \
  whitelabel/integrators/*.json
```

TODO(KAI-353): replace the python one-liner with `go run ./cmd/whitelabel-validate`.
