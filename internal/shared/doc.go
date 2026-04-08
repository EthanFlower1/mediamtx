// Package shared holds types, protocol definitions, and primitives that are
// common to both the Directory and Recorder roles of the Kaivue Recording
// Server.
//
// This is the only "common" import target permitted by the role boundary
// linter. If a piece of code is needed by both internal/directory and
// internal/recorder, it belongs here.
//
// Expected children (added incrementally as code migrates out of
// internal/nvr):
//
//   - shared/proto/v1   — Connect-Go .proto contracts for every inter-role
//     service. These are the source of truth for cross-role communication;
//     never hand-roll a parallel REST shape. Edits require the proto-lock
//     (see docs/proto-lock.md).
//   - shared/certmgr    — short-lived mTLS certificate manager backed by the
//     per-site step-ca. Listeners use tls.Config.GetCertificate so certs
//     rotate without restarts.
//   - shared/sidecar    — supervisor for managed sidecar processes
//     (Zitadel, MediaMTX). Sidecars bind to localhost or unix sockets only.
//   - shared/tsnetnode  — embedded tsnet/Headscale mesh node helpers used by
//     every component for inter-role traffic over mTLS.
//   - shared/errors     — stable error codes (per KAI-424) emitted across
//     role boundaries.
//
// Boundary rules (enforced by depguard, see .golangci.yml):
//
//   - Code under internal/shared MUST NOT import code under
//     internal/directory or internal/recorder. Shared code is a leaf in the
//     dependency graph with respect to the role packages.
//
// See docs/architecture/package-layout.md for the full plan.
package shared
