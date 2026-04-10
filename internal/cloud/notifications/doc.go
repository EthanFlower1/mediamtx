// Package notifications implements per-tenant notification channel
// configuration, user preference storage, and delivery routing for
// the Kaivue cloud platform (KAI-366).
//
// Supported channel types: email, push (APNs/FCM), SMS, webhook.
// Each tenant configures which channels are active and each user
// selects which event types they want delivered on which channels.
// The RouteNotification method resolves the delivery set for a given
// event and returns the list of channels + users to target.
package notifications
