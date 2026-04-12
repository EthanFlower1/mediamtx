// Package runtime — AllInOne boot sequence and subsystem interfaces.
//
// This file defines the booter interfaces that the Directory and Recorder
// packages must satisfy, and the AutoPair function that wires them
// together in ModeAllInOne without an HTTP round-trip.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
)

// ---------------------------------------------------------------------------
// Subsystem interfaces
// ---------------------------------------------------------------------------

// PairingTokenGenerator can mint an ephemeral pairing token.
// The Directory subsystem implements this.
type PairingTokenGenerator interface {
	GeneratePairingToken() (string, error)
}

// PairingTokenRedeemer can redeem a pairing token.
// The Recorder subsystem implements this.
type PairingTokenRedeemer interface {
	RedeemPairingToken(token string) error
}

// DirectoryBooter starts the Directory subsystem.
// The concrete implementation lives in internal/directory/boot.go
// (created by another agent).
//
// Boot receives an opaque config value (typically *conf.Conf) to avoid
// an import cycle between the runtime and conf packages. Implementations
// should type-assert to the concrete config type they need.
type DirectoryBooter interface {
	// Boot initialises and starts the Directory subsystem.
	// The cfg parameter is an opaque config (typically *conf.Conf).
	Boot(ctx context.Context, cfg any, logger *slog.Logger) error

	// PairingService returns the in-process pairing token generator
	// so that AllInOne mode can auto-pair without HTTP.
	PairingService() PairingTokenGenerator

	// Shutdown gracefully stops the Directory subsystem.
	Shutdown(ctx context.Context) error
}

// RecorderBooter starts the Recorder subsystem.
// The concrete implementation lives in internal/recorder/boot.go
// (created by another agent).
type RecorderBooter interface {
	// Boot initialises and starts the Recorder subsystem.
	// The cfg parameter is an opaque config (typically *conf.Conf).
	Boot(ctx context.Context, cfg any, logger *slog.Logger) error

	// PairingRedeemer returns the in-process pairing token redeemer
	// so that AllInOne mode can auto-pair without HTTP.
	PairingRedeemer() PairingTokenRedeemer

	// Shutdown gracefully stops the Recorder subsystem.
	Shutdown(ctx context.Context) error
}

// ---------------------------------------------------------------------------
// AllInOne boot helpers
// ---------------------------------------------------------------------------

// AutoPair performs in-process pairing of a Recorder to a Directory.
// It generates an ephemeral token via the Directory's pairing service
// and immediately redeems it on the Recorder side, all without any
// HTTP round-trip. Both sides are left paired in the shared SQLite DB.
//
// This is called once during ModeAllInOne startup after both subsystems
// have booted. On subsequent boots the Recorder state already records
// an existing pairing; callers should skip AutoPair in that case.
func AutoPair(ctx context.Context, dirPairing PairingTokenGenerator, recPairing PairingTokenRedeemer, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("all-in-one: generating ephemeral pairing token")

	token, err := dirPairing.GeneratePairingToken()
	if err != nil {
		return fmt.Errorf("auto-pair: generate token: %w", err)
	}

	logger.Info("all-in-one: redeeming pairing token on recorder")

	if err := recPairing.RedeemPairingToken(token); err != nil {
		return fmt.Errorf("auto-pair: redeem token: %w", err)
	}

	logger.Info("all-in-one: pairing complete")
	return nil
}

// StartAllInOne boots Directory, Recorder, and auto-pairs them.
// It is the canonical boot sequence for ModeAllInOne.
// The cfg parameter is an opaque config (typically *conf.Conf).
func StartAllInOne(ctx context.Context, dir DirectoryBooter, rec RecorderBooter, cfg any, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Start Directory first — it owns the pairing token table.
	logger.Info("all-in-one: booting directory subsystem")
	if err := dir.Boot(ctx, cfg, logger); err != nil {
		return fmt.Errorf("all-in-one: start directory: %w", err)
	}

	// 2. Start Recorder — it registers with the local Directory.
	logger.Info("all-in-one: booting recorder subsystem")
	if err := rec.Boot(ctx, cfg, logger); err != nil {
		// Best-effort shutdown of Directory on Recorder failure.
		_ = dir.Shutdown(ctx)
		return fmt.Errorf("all-in-one: start recorder: %w", err)
	}

	// 3. Auto-pair so there is no manual token exchange.
	if err := AutoPair(ctx, dir.PairingService(), rec.PairingRedeemer(), logger); err != nil {
		_ = rec.Shutdown(ctx)
		_ = dir.Shutdown(ctx)
		return fmt.Errorf("all-in-one: %w", err)
	}

	return nil
}
