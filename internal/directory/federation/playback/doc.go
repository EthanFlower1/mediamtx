// Package playback implements cross-site stream URL delegation (KAI-274).
//
// When a user at Directory A wants to play back a camera owned by Directory B,
// A asks B to mint a stream URL. B signs with its own JWKS key, pointing at
// its own Recorder/Gateway. The client then connects directly to B's edge;
// video bytes never proxy through A.
//
// The PlaybackDelegator is the central orchestrator:
//
//  1. Resolve camera_id -> owning peer from a federated catalog cache.
//  2. Call MintStreamURL on the owning peer's FederationPeerService.
//  3. Return the signed URL to the requesting client.
//
// Error handling covers: peer unreachable, permission denied by peer,
// camera not found in catalog, and unknown camera.
package playback
