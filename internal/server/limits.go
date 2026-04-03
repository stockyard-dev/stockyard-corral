package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
)

// Ed25519 public key for license validation (hex-encoded, matches Stockyard backend)
const publicKeyHex = "3af8f9593b3331c27994f1eeacf111c727ff6015016b0af44ed3ca6934d40b13"

type Limits struct {
	MaxEndpoints      int
	MaxEventsPerMonth int
	RetentionDays     int
	MaxForwardTargets int
	ReplayHistory     bool
	RetryDeliveries   bool
	EventSearch       bool
	ExportJSON        bool
	Tier              string
}

// FreeLimits returns the free tier limits (matches stockyard.dev/corral/).
func FreeLimits() Limits {
	return Limits{
		MaxEndpoints:      3,
		MaxEventsPerMonth: 1000,
		RetentionDays:     7,
		MaxForwardTargets: 1,
		ReplayHistory:     false,
		RetryDeliveries:   false,
		EventSearch:       false,
		ExportJSON:        false,
		Tier:              "free",
	}
}

// ProLimits returns the Pro tier limits (all features unlocked).
func ProLimits() Limits {
	return Limits{
		MaxEndpoints:      0, // unlimited
		MaxEventsPerMonth: 0, // unlimited
		RetentionDays:     90,
		MaxForwardTargets: 0, // unlimited
		ReplayHistory:     true,
		RetryDeliveries:   true,
		EventSearch:       true,
		ExportJSON:        true,
		Tier:              "pro",
	}
}

// DefaultLimits checks STOCKYARD_LICENSE_KEY and returns appropriate limits.
func DefaultLimits() Limits {
	key := os.Getenv("STOCKYARD_LICENSE_KEY")
	if key == "" {
		log.Printf("[license] No license key — running on free tier")
		log.Printf("[license] Set STOCKYARD_LICENSE_KEY to unlock Pro features")
		log.Printf("[license] Get a key at https://stockyard.dev/corral/")
		return FreeLimits()
	}
	if validateLicenseKey(key, "corral") {
		log.Printf("[license] Valid Pro license — all features unlocked")
		return ProLimits()
	}
	log.Printf("[license] Invalid license key — running on free tier")
	return FreeLimits()
}

// LimitReached returns true if the current count meets or exceeds the limit.
// A limit of 0 is treated as unlimited.
func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}

// validateLicenseKey verifies an Ed25519-signed license key.
// Format: SY-<base64url(payload)>.<base64url(signature)>
func validateLicenseKey(key, product string) bool {
	if !strings.HasPrefix(key, "SY-") {
		return false
	}
	key = key[3:]

	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	// Decode public key from hex
	pubKeyBytes, err := hexDecode(publicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), payloadBytes, sigBytes) {
		return false
	}

	// Parse payload to check product and expiry
	var payload struct {
		Product   string `json:"p"`
		Tier      string `json:"t"`
		ExpiresAt int64  `json:"x"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return false
	}

	// Check expiry
	if payload.ExpiresAt > 0 && time.Now().Unix() > payload.ExpiresAt {
		log.Printf("[license] License expired")
		return false
	}

	// Check product scope
	if payload.Product != "*" && payload.Product != "stockyard" && payload.Product != product {
		log.Printf("[license] License not valid for this product")
		return false
	}

	return true
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, os.ErrInvalid
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		high := hexVal(s[i])
		low := hexVal(s[i+1])
		if high == 255 || low == 255 {
			return nil, os.ErrInvalid
		}
		b[i/2] = high<<4 | low
	}
	return b, nil
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 255
}
