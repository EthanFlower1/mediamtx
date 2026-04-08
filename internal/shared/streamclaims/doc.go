// Package streamclaims implements the StreamClaims JWT format for Kaivue
// Recording Server stream access tokens (§9.1 of the multi-recording-server
// design spec, `docs/superpowers/specs/2026-04-07-multi-recording-server-design.md`).
//
// # Overview
//
// Every video-bearing URL served by a Recorder or Gateway carries a
// short-lived, single-use, scoped stream token. The token is a signed RS256
// JWT whose claims describe exactly one stream session: who is accessing it,
// from which tenant, on which camera, through which Recorder, using which
// protocol, and for what purpose (live / playback / snapshot / talkback).
//
// # Asymmetric minting
//
// Token issuance is asymmetric (§9.1):
//
//   - Cloud Directory signs tokens with its private RSA key via [Issuer].
//   - Recorders and Gateways verify tokens locally via a cached JWKS using
//     [Verifier]. No private key material ever leaves the cloud signing service.
//
// This split means Recorders can verify tokens entirely offline (after the
// initial JWKS fetch) and are therefore resilient to transient cloud
// connectivity loss. The [Verifier] caches the [jwk.Set] at construction
// time; callers are responsible for re-constructing or refreshing the verifier
// on key rotation.
//
// # Single-use nonce
//
// Each token carries a cryptographically random [StreamClaims.Nonce] field.
// Checking nonce uniqueness (bloom filter / Redis set) is the responsibility
// of KAI-257; this package only generates and embeds the nonce. Downstream
// verifiers MUST reject tokens whose nonce has already been seen.
//
// # Security invariants
//
//   - [Verifier] fails closed: any error returns a non-nil error and a nil
//     result. Callers may safely treat (nil, err) as deny.
//   - TTL is capped at [MaxTTL] (5 minutes) at sign time.
//   - Expired tokens are always rejected, even on clock-skew tolerance edges.
//   - [StreamKind] is a bitfield; a token MAY cover multiple stream kinds
//     (e.g. LIVE | AUDIO_TALKBACK), and the verifier enforces that the
//     requested kind is present.
//
// # Package boundary
//
// This package MUST NOT import from internal/directory/ or internal/recorder/.
// The depguard linter enforces this (KAI-236). Import only from the standard
// library, golang.org/x, and the approved third-party JWT/JWK libraries.
package streamclaims
