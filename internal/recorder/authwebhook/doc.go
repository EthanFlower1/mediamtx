// Package authwebhook implements a lightweight HTTP server that Raikada
// calls to authorize stream viewers. When Raikada is configured with
// authMethod: http and externalAuthenticationURL pointing at this server,
// every RTSP/WebRTC/HLS connection attempt is validated here.
//
// The webhook validates viewer tokens (JWT) and enforces tenant isolation:
// a viewer may only access camera paths assigned to their tenant on this
// Recorder.
//
// The webhook runs on a loopback listener (127.0.0.1) and is never exposed
// to the network — it only speaks to the co-located Raikada process.
package authwebhook
