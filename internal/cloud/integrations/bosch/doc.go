// Package bosch implements the Bosch B/G-Series alarm panel integration
// for the Kaivue NVR platform (KAI-406).
//
// The integration speaks the Bosch Mode2 binary protocol over TCP to a
// Bosch B-Series or G-Series intrusion panel, receives real-time alarm
// and zone events, and triggers camera actions (start recording, PTZ
// preset recall, webhook dispatch) in response.
//
// Architecture:
//
//   - Client        — TCP connection to the panel with Mode2 framing, heartbeat,
//                     and automatic reconnect.
//   - EventIngester — decodes panel event frames into normalized AlarmEvent
//                     structs and publishes them to the ActionRouter.
//   - ActionRouter  — matches alarm events against user-configured rules and
//                     dispatches camera actions (record, PTZ, webhook).
//   - Config        — panel connection + zone-to-camera mapping persisted in
//                     the cloud database per tenant.
//
// Multi-tenant invariant: every exported function or method that touches
// state requires a tenant ID; cross-tenant access is impossible by
// construction.
package bosch
