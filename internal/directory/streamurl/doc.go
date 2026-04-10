// Package streamurl contains the pure decision logic the Directory uses
// to mint stream URLs for clients (KAI-258).
//
// Given information about the calling client (its source IP, an optional
// is_lan hint), the target Recorder (its LAN CIDRs, gateway URL, cloud
// relay URL), and the tenant/recorder tier flags (Tier 2 Gateway enabled,
// Tier 3 Cloud Relay enabled), the package returns an *ordered* list of
// candidate endpoints the client should attempt.
//
// Ordering rules (highest preference first):
//
//  1. LAN endpoint, if the client is determined to be on the Recorder's
//     local network. "On the LAN" means either:
//       - the client's IP falls inside one of the Recorder's LAN CIDRs, or
//       - the request supplied an explicit is_lan=true hint AND a LAN URL
//         is configured on the Recorder.
//  2. Tier 2 Gateway endpoint, if Tier 2 is enabled on the recorder and
//     a gateway URL is configured. The gateway is a MediaMTX-fronted
//     streaming proxy on the customer's edge (KAI-261).
//  3. Tier 3 Cloud Relay endpoint, if Tier 3 is enabled and a cloud relay
//     URL is configured. This is the last-resort path that traverses the
//     Kaivue cloud edge.
//
// The function is pure: it performs no I/O, no DNS lookups, no clock or
// random reads. All inputs are passed by value, and the result is a
// freshly-allocated slice. This makes it trivial to table-test and to
// embed inside both the Directory's URL-minting handler and the upcoming
// Gateway role (KAI-261), which will share the same logic.
//
// Boundary rules:
//
//   - This package may not import internal/recorder/... (depguard, KAI-236).
//   - This package must not import any I/O, database, or transport packages.
//     It depends only on the Go standard library.
package streamurl
