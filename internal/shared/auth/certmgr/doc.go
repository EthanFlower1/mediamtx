// Package certmgr implements mTLS leaf-certificate lifecycle management for
// the Kaivue Recording Server components (Directory, Recorder, Gateway).
//
// Every component that participates in the per-site cluster mesh holds a leaf
// certificate signed by the site's embedded step-ca root. This package
// manages the full renewal lifecycle so that operators never need to manually
// rotate certs:
//
//   - ~24h cert lifetime, renewed when remaining lifetime drops below the
//     configurable RenewThreshold (default 8h).
//   - Hot reload via tls.Config.GetCertificate — the listener is never
//     restarted; new TLS handshakes transparently pick up the renewed cert.
//   - Renewal authentication: the current cert is presented to the CA as
//     client-auth for short-circuit renewal (no re-enrollment required).
//   - Re-enrollment fallback: if the current cert is expired (or the CA
//     rejects renewal), the Manager falls back to the pairing-time device
//     keypair and re-enrolls via the CA's JWK provisioner.
//
// # Fail semantics
//
// Cert management is fail-open for recording and fail-closed for auth:
//
//   - Renewal failures are alarmed (metrics + log) but never crash the process.
//   - The old cert keeps serving until it actually expires.
//   - A renewed cert that fails local verification is rejected; the old cert
//     continues to serve.
//
// # Usage
//
//	mgr, err := certmgr.New(certmgr.Config{
//	    CA:            myStepCAClient,        // certmgr.CAClient interface
//	    KeyStore:      myKeyStore,            // certmgr.KeyStore interface
//	    DeviceKey:     pairingTimePrivKey,    // crypto.PrivateKey
//	    SANs:          []string{"recorder-abc.kaivue.local"},
//	    Logger:        slog.Default(),
//	    Metrics:       certmgr.NewMetrics(),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	tlsConf := &tls.Config{GetCertificate: mgr.GetCertificate}
//	mgr.Start(ctx)
//	defer mgr.Shutdown(ctx)
//
// # Package boundary
//
// This package is in internal/shared/auth/certmgr/ and MUST NOT import from
// internal/directory/ or internal/recorder/. It may freely import from
// internal/shared/. The CAClient interface is the seam: callers (Directory,
// Recorder, Gateway) wire their concrete step-ca client in at construction
// time.
package certmgr
