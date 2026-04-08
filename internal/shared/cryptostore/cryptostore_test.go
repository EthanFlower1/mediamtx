package cryptostore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

// testMaster returns a deterministic master key derived from a placeholder
// string. Using sha256 keeps the test completely free of hardcoded secret
// material — the "master" is just hash("test-master-key-REPLACE_ME").
func testMaster(t *testing.T) []byte {
	t.Helper()
	h := sha256.Sum256([]byte("test-master-key-REPLACE_ME"))
	return h[:]
}

func testMasterAlt(t *testing.T) []byte {
	t.Helper()
	h := sha256.Sum256([]byte("test-master-key-REPLACE_ME-ALT"))
	return h[:]
}

func newTestStore(t *testing.T, info string) Cryptostore {
	t.Helper()
	s, err := NewFromMaster(testMaster(t), nil, info)
	if err != nil {
		t.Fatalf("NewFromMaster: %v", err)
	}
	return s
}

func TestRoundTrip(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	cases := [][]byte{
		nil,
		{},
		[]byte("a"),
		[]byte("rtsp://user:password@192.0.2.10/stream"),
		bytes.Repeat([]byte("x"), 4096),
	}
	for _, pt := range cases {
		ct, err := s.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		if len(ct) < HeaderSize+TagSize {
			t.Fatalf("ciphertext too short: %d", len(ct))
		}
		if ct[0] != FormatVersionV1 {
			t.Fatalf("version byte = 0x%02x, want 0x01", ct[0])
		}
		got, err := s.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("round trip mismatch: got %q, want %q", got, pt)
		}
	}
}

func TestNoncePerRow(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	pt := []byte("same-plaintext")
	ct1, err := s.Encrypt(pt)
	if err != nil {
		t.Fatal(err)
	}
	ct2, err := s.Encrypt(pt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ct1, ct2) {
		t.Fatal("expected different ciphertexts for same plaintext (random nonce)")
	}
	// nonce slice
	if bytes.Equal(ct1[1:1+NonceSize], ct2[1:1+NonceSize]) {
		t.Fatal("nonces collided across two calls")
	}
}

func TestTamperDetection(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	ct, err := s.Encrypt([]byte("top-secret"))
	if err != nil {
		t.Fatal(err)
	}

	// Flip a byte in each region and confirm Decrypt fails.
	regions := map[string]int{
		"version": 0,
		"nonce":   5,
		"body":    HeaderSize + 1,
		"tag":     len(ct) - 1,
	}
	for name, idx := range regions {
		bad := make([]byte, len(ct))
		copy(bad, ct)
		bad[idx] ^= 0x01
		_, err := s.Decrypt(bad)
		if err == nil {
			t.Fatalf("%s tamper: expected error, got nil", name)
		}
		// The version-byte case should return ErrUnsupportedVersion; others
		// should return ErrAuthFailed. Either way it must be a cryptostore
		// error and must not be nil.
		if name == "version" {
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("version tamper: want ErrUnsupportedVersion, got %v", err)
			}
		} else {
			if !errors.Is(err, ErrAuthFailed) {
				t.Fatalf("%s tamper: want ErrAuthFailed, got %v", name, err)
			}
		}
	}
}

func TestReservedVersionRejected(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	ct, err := s.Encrypt([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	ct[0] = FormatVersionReserved
	_, err = s.Decrypt(ct)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("reserved version: want ErrUnsupportedVersion, got %v", err)
	}
}

func TestShortCiphertextRejected(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	for _, n := range []int{0, 1, HeaderSize, HeaderSize + TagSize - 1} {
		_, err := s.Decrypt(make([]byte, n))
		if !errors.Is(err, ErrInvalidCiphertext) {
			t.Fatalf("len %d: want ErrInvalidCiphertext, got %v", n, err)
		}
	}
}

func TestDeriveSubkeyDeterministic(t *testing.T) {
	master := testMaster(t)
	k1, err := DeriveSubkey(master, nil, InfoRTSPCredentials)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := DeriveSubkey(master, nil, InfoRTSPCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatalf("HKDF not deterministic: %s vs %s", hex.EncodeToString(k1), hex.EncodeToString(k2))
	}
	if len(k1) != KeySize {
		t.Fatalf("subkey length = %d, want %d", len(k1), KeySize)
	}
}

func TestDeriveSubkeyInfoSeparation(t *testing.T) {
	master := testMaster(t)
	infos := []string{
		InfoRTSPCredentials,
		InfoFaceVault,
		InfoPairingTokens,
		InfoFederationRoot,
		InfoZitadelBootstrap,
	}
	seen := map[string]string{}
	for _, info := range infos {
		k, err := DeriveSubkey(master, nil, info)
		if err != nil {
			t.Fatal(err)
		}
		h := hex.EncodeToString(k)
		if prev, ok := seen[h]; ok {
			t.Fatalf("info %q and %q produced same subkey", info, prev)
		}
		seen[h] = info
	}
}

func TestCrossInfoDoesNotDecrypt(t *testing.T) {
	master := testMaster(t)
	rtsp, _ := NewFromMaster(master, nil, InfoRTSPCredentials)
	face, _ := NewFromMaster(master, nil, InfoFaceVault)
	ct, err := rtsp.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := face.Decrypt(ct); err == nil {
		t.Fatal("face store should not decrypt rtsp ciphertext")
	}
}

func TestRotateKeyOnInstance(t *testing.T) {
	oldKey, _ := DeriveSubkey(testMaster(t), nil, InfoRTSPCredentials)
	newKey, _ := DeriveSubkey(testMasterAlt(t), nil, InfoRTSPCredentials)

	s, err := New(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := s.Encrypt([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := s.Decrypt(ct)
	if err != nil || string(pt) != "hello" {
		t.Fatalf("pre-rotate decrypt failed: %v %q", err, pt)
	}

	// Rotate in place.
	if err := s.RotateKey(oldKey, newKey); err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	// Old ciphertext must no longer decrypt with the new key.
	if _, err := s.Decrypt(ct); err == nil {
		t.Fatal("expected old ciphertext to fail under new key")
	}
	// New ciphertext round-trips.
	ct2, err := s.Encrypt([]byte("world"))
	if err != nil {
		t.Fatal(err)
	}
	pt2, err := s.Decrypt(ct2)
	if err != nil || string(pt2) != "world" {
		t.Fatalf("post-rotate decrypt failed: %v %q", err, pt2)
	}
	// Wrong old key is rejected.
	bad := make([]byte, KeySize)
	if err := s.RotateKey(bad, newKey); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("rotate with wrong old key: want ErrInvalidKey, got %v", err)
	}
}

func TestRotateValueFlow(t *testing.T) {
	oldStore, _ := NewFromMaster(testMaster(t), nil, InfoRTSPCredentials)
	newStore, _ := NewFromMaster(testMasterAlt(t), nil, InfoRTSPCredentials)

	ct, err := oldStore.Encrypt([]byte("password"))
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := RotateValue(oldStore, newStore, ct)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldStore.Decrypt(rotated); err == nil {
		t.Fatal("rotated ct should not decrypt under old store")
	}
	pt, err := newStore.Decrypt(rotated)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "password" {
		t.Fatalf("plaintext mismatch after rotation: %q", pt)
	}
}

func TestNonceUniqueness(t *testing.T) {
	s := newTestStore(t, InfoRTSPCredentials)
	seen := make(map[string]struct{}, 10000)
	pt := []byte("x")
	for i := 0; i < 10000; i++ {
		ct, err := s.Encrypt(pt)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		n := string(ct[1 : 1+NonceSize])
		if _, dup := seen[n]; dup {
			t.Fatalf("nonce collision on iter %d", i)
		}
		seen[n] = struct{}{}
	}
}

func TestInvalidInputs(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("New(nil): want ErrInvalidKey, got %v", err)
	}
	if _, err := New(make([]byte, 16)); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("New(16 bytes): want ErrInvalidKey, got %v", err)
	}
	if _, err := NewFromMaster(nil, nil, "x"); !errors.Is(err, ErrEmptyMaster) {
		t.Fatalf("NewFromMaster empty master: want ErrEmptyMaster, got %v", err)
	}
	if _, err := NewFromMaster([]byte("m"), nil, ""); !errors.Is(err, ErrEmptyInfo) {
		t.Fatalf("NewFromMaster empty info: want ErrEmptyInfo, got %v", err)
	}
}

func TestRotateColumn(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE cameras (id INTEGER PRIMARY KEY, rtsp_credentials_encrypted BLOB)`); err != nil {
		t.Fatal(err)
	}

	oldStore, _ := NewFromMaster(testMaster(t), nil, InfoRTSPCredentials)
	newStore, _ := NewFromMaster(testMasterAlt(t), nil, InfoRTSPCredentials)

	plaintexts := map[int]string{
		1: "rtsp://alice:hunter2@10.0.0.1/stream1",
		2: "rtsp://bob:sw0rdfish@10.0.0.2/stream2",
		3: "rtsp://carol:letmein@10.0.0.3/stream3",
	}
	for id, pt := range plaintexts {
		ct, err := oldStore.Encrypt([]byte(pt))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO cameras (id, rtsp_credentials_encrypted) VALUES (?, ?)`, id, ct); err != nil {
			t.Fatal(err)
		}
	}
	// NULL row must be ignored.
	if _, err := db.Exec(`INSERT INTO cameras (id, rtsp_credentials_encrypted) VALUES (?, NULL)`, 99); err != nil {
		t.Fatal(err)
	}

	n, err := RotateColumn(context.Background(), db, "cameras", "rtsp_credentials_encrypted", oldStore, newStore, RotateColumnOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("RotateColumn: %v", err)
	}
	if n != len(plaintexts) {
		t.Fatalf("rotated %d rows, want %d", n, len(plaintexts))
	}

	// Verify each row decrypts under the new store and not the old one.
	for id, want := range plaintexts {
		var ct []byte
		if err := db.QueryRow(`SELECT rtsp_credentials_encrypted FROM cameras WHERE id = ?`, id).Scan(&ct); err != nil {
			t.Fatal(err)
		}
		if _, err := oldStore.Decrypt(ct); err == nil {
			t.Fatalf("row %d still decrypts under old key", id)
		}
		pt, err := newStore.Decrypt(ct)
		if err != nil {
			t.Fatalf("row %d new decrypt: %v", id, err)
		}
		if string(pt) != want {
			t.Fatalf("row %d plaintext = %q, want %q", id, pt, want)
		}
	}

	// Second rotation call should be a no-op (zero progress).
	n2, err := RotateColumn(context.Background(), db, "cameras", "rtsp_credentials_encrypted", oldStore, newStore, RotateColumnOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("second RotateColumn: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second rotation rotated %d rows, want 0", n2)
	}
}

// Ensure the Cryptostore interface is satisfied by the concrete type at
// compile time. Consumers (e.g. KAI-250 recorder/state) depend on this.
var _ Cryptostore = (*aesGCM)(nil)

// Example compile-time sanity: prevent accidental changes to HeaderSize.
func TestHeaderLayout(t *testing.T) {
	if HeaderSize != 13 {
		t.Fatalf("HeaderSize = %d, want 13 (1 version + 12 nonce)", HeaderSize)
	}
	s := newTestStore(t, InfoRTSPCredentials)
	ct, _ := s.Encrypt([]byte("abc"))
	if got := fmt.Sprintf("%d", len(ct)); got != "32" {
		// 1 + 12 + 3 + 16 = 32
		t.Fatalf("len(ct for 3-byte plaintext) = %s, want 32", got)
	}
}
