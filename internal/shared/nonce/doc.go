// Package nonce provides a sliding-window bloom filter for detecting
// single-use nonce reuse across the Kaivue Recording Server.
//
// # Why this exists
//
// The Recorder's HMAC auth webhook (KAI-260) and the Gateway (KAI-261)
// both need to reject replayed requests by tracking nonces seen within a
// short time window. Storing every nonce in a map is unbounded; storing
// them in SQLite is too slow for the request path. A bloom filter with a
// known false-positive rate is the standard primitive for this — the
// security tradeoff is that an attacker may, with probability < FPR, get
// a fresh nonce mistakenly rejected as replayed (a denial of service on
// themselves), but a replayed nonce is *always* detected.
//
// # Sizing
//
// For capacity n=1,000,000 nonces and false-positive rate p=0.001:
//
//	m = -n * ln(p) / (ln(2)^2) ≈ 14,377,587 bits ≈ 1.8 MB
//	k = (m/n) * ln(2)          ≈ 10 hash functions
//
// The sliding window is implemented as two rotating bloom windows; at any
// instant we query both (logical OR) and insert into the active one.
// Every TTL/2 we rotate: the active window becomes the "previous" window
// and a fresh empty window becomes active. After TTL elapses, the oldest
// window is fully discarded — old nonces age out automatically. Total
// memory is therefore ~3.6 MB for the default sizing, which is acceptable.
//
// # Hard rules
//
//   - This is a security primitive. Any change to hashing, sizing, or
//     rotation semantics MUST be reviewed by lead-security before merge.
//   - The filter is fail-closed: a nil receiver or a closed Filter treats
//     every nonce as already-seen (wasNew=false), so replay protection
//     never silently degrades to "allow everything".
//   - All exported methods are safe for concurrent use.
package nonce
