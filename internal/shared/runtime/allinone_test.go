package runtime

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockPairingGen struct {
	token string
	err   error
}

func (m *mockPairingGen) GeneratePairingToken() (string, error) {
	return m.token, m.err
}

type mockPairingRedeem struct {
	redeemed string
	err      error
}

func (m *mockPairingRedeem) RedeemPairingToken(token string) error {
	m.redeemed = token
	return m.err
}

type mockDirectoryBooter struct {
	booted     bool
	bootErr    error
	shutdownFn func()
	pairing    *mockPairingGen
}

func (m *mockDirectoryBooter) Boot(_ context.Context, _ any, _ *slog.Logger) error {
	m.booted = true
	return m.bootErr
}

func (m *mockDirectoryBooter) PairingService() PairingTokenGenerator {
	return m.pairing
}

func (m *mockDirectoryBooter) Shutdown(_ context.Context) error {
	if m.shutdownFn != nil {
		m.shutdownFn()
	}
	return nil
}

type mockRecorderBooter struct {
	booted     bool
	bootErr    error
	shutdownFn func()
	redeemer   *mockPairingRedeem
}

func (m *mockRecorderBooter) Boot(_ context.Context, _ any, _ *slog.Logger) error {
	m.booted = true
	return m.bootErr
}

func (m *mockRecorderBooter) PairingRedeemer() PairingTokenRedeemer {
	return m.redeemer
}

func (m *mockRecorderBooter) Shutdown(_ context.Context) error {
	if m.shutdownFn != nil {
		m.shutdownFn()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAutoPairSuccess(t *testing.T) {
	gen := &mockPairingGen{token: "tok-abc"}
	red := &mockPairingRedeem{}

	err := AutoPair(context.Background(), gen, red, nil)
	require.NoError(t, err)
	require.Equal(t, "tok-abc", red.redeemed)
}

func TestAutoPairGenerateError(t *testing.T) {
	gen := &mockPairingGen{err: errors.New("gen fail")}
	red := &mockPairingRedeem{}

	err := AutoPair(context.Background(), gen, red, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "generate token")
	require.Empty(t, red.redeemed)
}

func TestAutoPairRedeemError(t *testing.T) {
	gen := &mockPairingGen{token: "tok-xyz"}
	red := &mockPairingRedeem{err: errors.New("redeem fail")}

	err := AutoPair(context.Background(), gen, red, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "redeem token")
}

func TestStartAllInOneSuccess(t *testing.T) {
	gen := &mockPairingGen{token: "tok-aio"}
	red := &mockPairingRedeem{}
	dir := &mockDirectoryBooter{pairing: gen}
	rec := &mockRecorderBooter{redeemer: red}

	err := StartAllInOne(context.Background(), dir, rec, nil, nil)
	require.NoError(t, err)
	require.True(t, dir.booted)
	require.True(t, rec.booted)
	require.Equal(t, "tok-aio", red.redeemed)
}

func TestStartAllInOneDirectoryBootFails(t *testing.T) {
	dir := &mockDirectoryBooter{
		bootErr: errors.New("dir boom"),
		pairing: &mockPairingGen{},
	}
	rec := &mockRecorderBooter{redeemer: &mockPairingRedeem{}}

	err := StartAllInOne(context.Background(), dir, rec, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start directory")
	require.False(t, rec.booted, "recorder should not boot if directory fails")
}

func TestStartAllInOneRecorderBootFails(t *testing.T) {
	var dirShutdown bool
	dir := &mockDirectoryBooter{
		pairing:    &mockPairingGen{},
		shutdownFn: func() { dirShutdown = true },
	}
	rec := &mockRecorderBooter{
		bootErr:  errors.New("rec boom"),
		redeemer: &mockPairingRedeem{},
	}

	err := StartAllInOne(context.Background(), dir, rec, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start recorder")
	require.True(t, dirShutdown, "directory should be shut down on recorder failure")
}

func TestStartAllInOneAutoPairFails(t *testing.T) {
	var dirShutdown, recShutdown bool
	dir := &mockDirectoryBooter{
		pairing:    &mockPairingGen{err: errors.New("pair boom")},
		shutdownFn: func() { dirShutdown = true },
	}
	rec := &mockRecorderBooter{
		redeemer:   &mockPairingRedeem{},
		shutdownFn: func() { recShutdown = true },
	}

	err := StartAllInOne(context.Background(), dir, rec, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auto-pair")
	require.True(t, dirShutdown, "directory should be shut down on pair failure")
	require.True(t, recShutdown, "recorder should be shut down on pair failure")
}
