package certmgr

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"time"
)

// CAClient is the minimal interface this package requires from a step-ca
// client. The concrete implementation lives in internal/directory/pki/
// (Directory role) or can be provided by any caller that wraps the cluster CA.
// Callers inject the concrete implementation at construction time — this
// package never imports internal/directory/ or internal/recorder/.
type CAClient interface {
	// Renew uses the supplied current certificate + key as mutual-TLS client
	// credentials to obtain a renewed certificate from the CA. The returned
	// cert shares the same identity (SANs, subject) but has a fresh NotAfter.
	//
	// Callers MUST treat the returned *tls.Certificate as immutable.
	Renew(ctx context.Context, current *tls.Certificate) (*tls.Certificate, error)

	// ReEnroll performs a first-enrollment style request using the device
	// keypair generated at pairing time. This is the fallback path when the
	// leaf cert is expired or when Renew fails with a certificate-expired /
	// certificate-unknown alert.
	//
	// sans contains the DNS names / IP SANs to embed in the new leaf.
	ReEnroll(ctx context.Context, deviceKey crypto.PrivateKey, sans []string) (*tls.Certificate, error)

	// RootPool returns the current trust pool for verifying renewed leaves.
	// Called after each renewal to confirm the new cert chains to the site
	// root before hot-swapping it in.
	RootPool() *x509.CertPool
}

// KeyStore persists and loads the current leaf cert so that a process restart
// picks up the most-recently issued cert rather than re-enrolling every boot.
type KeyStore interface {
	// Load attempts to load the most recently persisted leaf cert. Returns
	// (nil, nil) if no cert is stored yet. Returns a non-nil error only if
	// storage is readable but corrupt.
	Load(ctx context.Context) (*tls.Certificate, error)

	// Save persists the new leaf cert, replacing any previously stored cert.
	// Called after each successful renewal or re-enrollment.
	Save(ctx context.Context, cert *tls.Certificate) error
}

// ClockFunc is the functional form of a clock — simply a func() time.Time.
// The Manager accepts a ClockFunc so tests can fast-forward time without any
// interface boilerplate.
type ClockFunc func() time.Time
