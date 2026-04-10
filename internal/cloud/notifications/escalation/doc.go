// Package escalation implements customer-defined escalation chains with
// per-alert state-machine tracking, acknowledgement handling, and
// PagerDuty fallback (KAI-372).
//
// An escalation chain is a named, ordered sequence of steps. Each step
// targets a user, group, or PagerDuty integration via a specific channel
// (email, push, SMS, webhook, pagerduty). If the current tier is not
// acknowledged within its timeout, the state machine advances to the next
// tier. The final tier may use a PagerDuty channel for on-call rotation.
//
// State machine per alert:
//
//	pending -> notified -> (ack? -> acknowledged -> resolved)
//	                    -> (timeout? -> next tier notified -> ...)
//	                    -> (last tier timeout? -> pagerduty_fallback | exhausted)
package escalation
