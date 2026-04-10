// Package federation implements the FederationPeerService, which exposes
// Directory-to-Directory RPCs over Connect-Go with mTLS authentication.
//
// This is the service scaffold for KAI-464. Ping and GetJWKS are fully
// implemented; the remaining RPCs (ListUsers, ListGroups, ListCameras,
// SearchRecordings, MintStreamURL) return Unimplemented and will be filled
// in by KAI-465 and KAI-466.
package federation
