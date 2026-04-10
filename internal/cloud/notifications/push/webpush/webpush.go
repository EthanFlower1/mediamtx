package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"
)

// Config configures the Web Push adapter.
type Config struct {
	VAPIDPublicKey  string // base64url-encoded
	VAPIDPrivateKey string // base64url-encoded
	Subject         string // mailto: or https: contact URL

	HTTPClient *http.Client
}

// Subscription represents a browser push subscription (the JSON object
// returned by PushManager.subscribe() in the browser).
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"` // base64url
		Auth   string `json:"auth"`   // base64url
	} `json:"keys"`
}

// Channel is the Web Push delivery channel adapter.
type Channel struct {
	vapidPub  *ecdsa.PublicKey
	vapidPriv *ecdsa.PrivateKey
	subject   string
	client    *http.Client

	sent    int64
	failed  int64
	removed int64
}

// compile-time interface check
var _ notifications.DeliveryChannel = (*Channel)(nil)

// New constructs a ready-to-use Web Push channel.
func New(cfg Config) (*Channel, error) {
	if cfg.VAPIDPublicKey == "" {
		return nil, fmt.Errorf("webpush: vapid_public_key required")
	}
	if cfg.VAPIDPrivateKey == "" {
		return nil, fmt.Errorf("webpush: vapid_private_key required")
	}
	if cfg.Subject == "" {
		return nil, fmt.Errorf("webpush: subject required")
	}

	privBytes, err := base64.RawURLEncoding.DecodeString(cfg.VAPIDPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: decode private key: %w", err)
	}
	pubBytes, err := base64.RawURLEncoding.DecodeString(cfg.VAPIDPublicKey)
	if err != nil {
		return nil, fmt.Errorf("webpush: decode public key: %w", err)
	}

	priv, pub, err := decodeVAPIDKeys(privBytes, pubBytes)
	if err != nil {
		return nil, fmt.Errorf("webpush: %w", err)
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Channel{
		vapidPub:  pub,
		vapidPriv: priv,
		subject:   cfg.Subject,
		client:    client,
	}, nil
}

// Type implements DeliveryChannel.
func (c *Channel) Type() notifications.ChannelType {
	return notifications.ChannelPush
}

// Send implements DeliveryChannel. The msg.Target should be a JSON-encoded
// Subscription object.
func (c *Channel) Send(ctx context.Context, msg notifications.Message) (notifications.DeliveryResult, error) {
	var sub Subscription
	if err := json.Unmarshal([]byte(msg.Target), &sub); err != nil {
		return notifications.DeliveryResult{
			Target:       msg.Target,
			State:        notifications.StateFailed,
			ErrorMessage: fmt.Sprintf("parse subscription: %v", err),
		}, nil
	}

	payload, err := json.Marshal(map[string]interface{}{
		"title":    msg.Title,
		"body":     msg.Body,
		"data":     msg.Data,
		"icon":     msg.ImageURL,
		"badge":    msg.Badge,
	})
	if err != nil {
		return notifications.DeliveryResult{
			Target:       sub.Endpoint,
			State:        notifications.StateFailed,
			ErrorMessage: fmt.Sprintf("marshal payload: %v", err),
		}, nil
	}

	encrypted, err := encryptPayload(sub, payload)
	if err != nil {
		return notifications.DeliveryResult{
			Target:       sub.Endpoint,
			State:        notifications.StateFailed,
			ErrorMessage: fmt.Sprintf("encrypt: %v", err),
		}, nil
	}

	vapidHeader, err := c.generateVAPIDAuth(sub.Endpoint)
	if err != nil {
		return notifications.DeliveryResult{
			Target:       sub.Endpoint,
			State:        notifications.StateFailed,
			ErrorMessage: fmt.Sprintf("vapid: %v", err),
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted.body))
	if err != nil {
		return notifications.DeliveryResult{
			Target:       sub.Endpoint,
			State:        notifications.StateFailed,
			ErrorMessage: err.Error(),
		}, nil
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", fmt.Sprintf("%d", int(msg.TTL.Seconds())))
	req.Header.Set("Authorization", vapidHeader)
	if msg.Priority == "high" {
		req.Header.Set("Urgency", "high")
	} else {
		req.Header.Set("Urgency", "normal")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		atomic.AddInt64(&c.failed, 1)
		return notifications.DeliveryResult{
			Target:       sub.Endpoint,
			State:        notifications.StateFailed,
			ErrorMessage: err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	return c.parseResponse(resp, sub.Endpoint)
}

// BatchSend implements DeliveryChannel.
func (c *Channel) BatchSend(ctx context.Context, msg notifications.BatchMessage) ([]notifications.DeliveryResult, error) {
	results := make([]notifications.DeliveryResult, len(msg.Targets))
	for i, tgt := range msg.Targets {
		single := notifications.Message{
			MessageID:   fmt.Sprintf("%s-%d", msg.MessageID, i),
			TenantID:    msg.TenantID,
			UserID:      tgt.UserID,
			Target:      tgt.DeviceToken,
			Platform:    tgt.Platform,
			Title:       msg.Title,
			Body:        msg.Body,
			Data:        msg.Data,
			ImageURL:    msg.ImageURL,
			Priority:    msg.Priority,
			TTL:         msg.TTL,
			CollapseKey: msg.CollapseKey,
			Badge:       msg.Badge,
			Sound:       msg.Sound,
		}
		result, err := c.Send(ctx, single)
		if err != nil {
			results[i] = notifications.DeliveryResult{
				Target:       tgt.DeviceToken,
				State:        notifications.StateFailed,
				ErrorMessage: err.Error(),
			}
			continue
		}
		results[i] = result
	}
	return results, nil
}

// CheckHealth implements DeliveryChannel. Validates that VAPID keys can
// produce a valid JWT.
func (c *Channel) CheckHealth(ctx context.Context) error {
	_, err := c.generateVAPIDAuth("https://health.check.example.com")
	if err != nil {
		return fmt.Errorf("webpush health: %w", err)
	}
	return nil
}

// Stats returns cumulative counters.
func (c *Channel) Stats() (sent, failed, removed int64) {
	return atomic.LoadInt64(&c.sent), atomic.LoadInt64(&c.failed), atomic.LoadInt64(&c.removed)
}

// ---------- internal ----------

func (c *Channel) generateVAPIDAuth(audience string) (string, error) {
	// Extract origin from endpoint URL
	parts := strings.SplitN(audience, "/", 4)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid endpoint URL")
	}
	origin := parts[0] + "//" + parts[2]

	now := time.Now()
	claims := jwt.MapClaims{
		"aud": origin,
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": c.subject,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	signed, err := token.SignedString(c.vapidPriv)
	if err != nil {
		return "", err
	}

	pubBytes := elliptic.Marshal(c.vapidPub.Curve, c.vapidPub.X, c.vapidPub.Y)
	pubEncoded := base64.RawURLEncoding.EncodeToString(pubBytes)

	return fmt.Sprintf("vapid t=%s, k=%s", signed, pubEncoded), nil
}

func (c *Channel) parseResponse(resp *http.Response, target string) (notifications.DeliveryResult, error) {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		atomic.AddInt64(&c.sent, 1)
		location := resp.Header.Get("Location")
		return notifications.DeliveryResult{
			Target:     target,
			State:      notifications.StateDelivered,
			PlatformID: location,
		}, nil

	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		atomic.AddInt64(&c.removed, 1)
		return notifications.DeliveryResult{
			Target:            target,
			State:             notifications.StateUnreachable,
			ErrorCode:         fmt.Sprintf("%d", resp.StatusCode),
			ShouldRemoveToken: true,
		}, nil

	case resp.StatusCode == http.StatusTooManyRequests:
		return notifications.DeliveryResult{
			Target:    target,
			State:     notifications.StateThrottled,
			ErrorCode: "429",
		}, nil

	default:
		atomic.AddInt64(&c.failed, 1)
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return notifications.DeliveryResult{
			Target:       target,
			State:        notifications.StateFailed,
			ErrorCode:    fmt.Sprintf("%d", resp.StatusCode),
			ErrorMessage: string(respBody),
		}, nil
	}
}

// decodeVAPIDKeys builds ECDSA key objects from raw VAPID key bytes.
func decodeVAPIDKeys(privBytes, pubBytes []byte) (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	curve := elliptic.P256()

	x, y := elliptic.Unmarshal(curve, pubBytes)
	if x == nil {
		return nil, nil, fmt.Errorf("invalid VAPID public key")
	}

	pub := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
	priv := &ecdsa.PrivateKey{
		PublicKey: *pub,
		D:        new(big.Int).SetBytes(privBytes),
	}

	return priv, pub, nil
}

// encryptedPayload holds the output of Web Push encryption.
type encryptedPayload struct {
	body []byte
}

// encryptPayload implements RFC 8291 (Message Encryption for Web Push)
// using aes128gcm content encoding.
func encryptPayload(sub Subscription, plaintext []byte) (*encryptedPayload, error) {
	// Decode subscriber's public key and auth secret
	p256dhBytes, err := base64.RawURLEncoding.DecodeString(sub.Keys.P256dh)
	if err != nil {
		return nil, fmt.Errorf("decode p256dh: %w", err)
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(sub.Keys.Auth)
	if err != nil {
		return nil, fmt.Errorf("decode auth: %w", err)
	}

	// Generate ephemeral key pair
	ephPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}
	ephPub := ephPriv.PublicKey()

	// Import subscriber's public key
	subPub, err := ecdh.P256().NewPublicKey(p256dhBytes)
	if err != nil {
		return nil, fmt.Errorf("import subscriber key: %w", err)
	}

	// ECDH shared secret
	sharedSecret, err := ephPriv.ECDH(subPub)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	// Key derivation per RFC 8291
	// IKM = HKDF-SHA256(auth_secret, shared_secret, "WebPush: info" || 0x00 || client_pub || server_pub || 0x01)
	ikm := deriveIKM(authSecret, sharedSecret, p256dhBytes, ephPub.Bytes())

	// Salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// PRK
	prkReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: aes128gcm\x00"))
	key := make([]byte, 16)
	if _, err := io.ReadFull(prkReader, key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	nonceReader := hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: nonce\x00"))
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(nonceReader, nonce); err != nil {
		return nil, fmt.Errorf("derive nonce: %w", err)
	}

	// Pad the plaintext (mandatory delimiter byte 0x02 for final record)
	padded := append(plaintext, 0x02)

	// Encrypt with AES-128-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	// Build aes128gcm content-coding header:
	// salt (16) || rs (4) || idlen (1) || keyid (65 for uncompressed P-256)
	ephPubBytes := ephPub.Bytes()
	rs := uint32(4096)
	var header bytes.Buffer
	header.Write(salt)
	binary.Write(&header, binary.BigEndian, rs)
	header.WriteByte(byte(len(ephPubBytes)))
	header.Write(ephPubBytes)
	header.Write(ciphertext)

	return &encryptedPayload{body: header.Bytes()}, nil
}

// deriveIKM derives the input keying material per RFC 8291 section 3.4.
func deriveIKM(authSecret, sharedSecret, clientPub, serverPub []byte) []byte {
	info := buildInfo("WebPush: info", clientPub, serverPub)
	r := hkdf.New(sha256.New, sharedSecret, authSecret, info)
	ikm := make([]byte, 32)
	io.ReadFull(r, ikm)
	return ikm
}

func buildInfo(infoType string, clientPub, serverPub []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(infoType)
	buf.WriteByte(0x00)
	buf.Write(clientPub)
	buf.Write(serverPub)
	return buf.Bytes()
}
