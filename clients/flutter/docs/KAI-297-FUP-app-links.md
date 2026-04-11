# KAI-297-FUP: Migrate to App Links / Universal Links

**Status**: Pending Linear filing (token expired 2026-04-08)
**Priority**: High (must land before pen test window 2026-08-01)
**Owner**: lead-flutter
**Related**: KAI-297, KAI-354, KAI-305

## Background

Per lead-security's KAI-297 review, the current Flutter login flow uses custom URL
schemes (`com.kaivue.app://callback`) for OIDC redirect. Custom schemes are vulnerable
to scheme hijacking on Android (any app can register the same scheme).

## Requirements

### Android: App Links
- Register intent filter with `autoVerify=true` in AndroidManifest.xml
- Host Digital Asset Links (`.well-known/assetlinks.json`) on each white-label brand domain
- DAL must contain SHA-256 fingerprint of the signing certificate

### iOS: Universal Links
- Configure Associated Domains entitlement (`applinks:<brand-domain>`)
- Host Apple App Site Association (`.well-known/apple-app-site-association`) on each brand domain
- AASA must list the app's bundle ID and allowed paths

### Per-Brand Configuration
- KAI-354 (mobile build pipeline) generates DAL/AASA per brand domain
- KAI-305 (BrandConfig) provides the brand domain registry
- flutter_appauth redirect URI switches from custom scheme to Universal Links URI

### Acceptance Criteria
- [ ] Android App Links verified via DAL on brand domain
- [ ] iOS Universal Links verified via AASA on brand domain
- [ ] Custom URL scheme removed from production builds
- [ ] flutter_appauth configured with Universal Links redirect URI
- [ ] Per-brand DAL/AASA generation integrated into build pipeline (KAI-354)
- [ ] Regression tests for redirect flow on both platforms

## Deadline

2026-08-01 (pen test window opens)
