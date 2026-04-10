// Package federation implements the FederationPeerService, which exposes
// Directory-to-Directory RPCs over Connect-Go with mTLS authentication.
//
// KAI-464 laid the scaffold with Ping and GetJWKS. KAI-465 adds the three
// catalog RPCs (ListUsers, ListGroups, ListCameras), each filtered by Casbin
// grants for the requesting peer using the "federation:<peer_directory_id>"
// subject prefix. SearchRecordings and MintStreamURL remain Unimplemented
// and will be filled in by KAI-466.
package federation
