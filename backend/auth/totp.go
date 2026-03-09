package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPManager handles TOTP secret storage, encryption, and validation.
type TOTPManager struct {
	mu       sync.RWMutex
	filePath string
	encKey   []byte // 32-byte AES-256 key derived from SESSION_SECRET
	data     *totpData
}

type totpData struct {
	EncryptedSecret string `json:"encrypted_secret"`
	Enabled         bool   `json:"enabled"`
	FailedAttempts  int    `json:"failed_attempts"`
	LockedUntil     int64  `json:"locked_until,omitempty"` // unix timestamp
}

// NewTOTPManager creates a manager that stores TOTP secrets encrypted at the given path.
// The encryption key is derived from SESSION_SECRET.
func NewTOTPManager(dataDir string) (*TOTPManager, error) {
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		return nil, errors.New("SESSION_SECRET is required for 2FA support")
	}

	// Derive 32-byte key from SESSION_SECRET
	hash := sha256.Sum256([]byte(secret))

	path := filepath.Join(dataDir, "totp_secret.json")

	tm := &TOTPManager{
		filePath: path,
		encKey:   hash[:],
	}

	// Load existing data if present
	if err := tm.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return tm, nil
}

// IsEnabled returns whether 2FA is currently enabled.
func (tm *TOTPManager) IsEnabled() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.data != nil && tm.data.Enabled
}

// GenerateSecret creates a new TOTP key and returns the OTP key (for QR code generation).
// The secret is NOT saved until ConfirmSetup is called with a valid code.
func (tm *TOTPManager) GenerateSecret(issuer, accountName string) (*otp.Key, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, err
	}
	return key, nil
}

// ConfirmSetup validates the TOTP code against the provided secret, and if valid,
// encrypts and saves the secret to disk.
func (tm *TOTPManager) ConfirmSetup(secret, code string) error {
	valid := totp.Validate(code, secret)
	if !valid {
		return errors.New("invalid TOTP code")
	}

	encrypted, err := tm.encrypt(secret)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.data = &totpData{
		EncryptedSecret: encrypted,
		Enabled:         true,
	}
	return tm.save()
}

// Validate checks a TOTP code. Returns an error if invalid, locked out, or 2FA not enabled.
func (tm *TOTPManager) Validate(code string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.data == nil || !tm.data.Enabled {
		return errors.New("2FA is not enabled")
	}

	// Check lockout
	if tm.data.LockedUntil > 0 && time.Now().Unix() < tm.data.LockedUntil {
		remaining := tm.data.LockedUntil - time.Now().Unix()
		return errors.New("too many failed attempts, try again in " + formatDuration(remaining))
	}

	secret, err := tm.decrypt(tm.data.EncryptedSecret)
	if err != nil {
		return errors.New("internal error decrypting TOTP secret")
	}

	valid := totp.Validate(code, secret)
	if !valid {
		tm.data.FailedAttempts++
		if tm.data.FailedAttempts >= 5 {
			tm.data.LockedUntil = time.Now().Add(15 * time.Minute).Unix()
			tm.data.FailedAttempts = 0
			_ = tm.save()
			return errors.New("too many failed attempts, locked for 15 minutes")
		}
		_ = tm.save()
		return errors.New("invalid TOTP code")
	}

	// Reset failed attempts on success
	tm.data.FailedAttempts = 0
	tm.data.LockedUntil = 0
	_ = tm.save()
	return nil
}

// Disable removes the TOTP secret after validating the current code.
func (tm *TOTPManager) Disable(code string) error {
	if err := tm.Validate(code); err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.data = &totpData{Enabled: false}
	return tm.save()
}

func (tm *TOTPManager) load() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	raw, err := os.ReadFile(tm.filePath)
	if err != nil {
		return err
	}
	var d totpData
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}
	tm.data = &d
	return nil
}

func (tm *TOTPManager) save() error {
	// Ensure directory exists
	dir := filepath.Dir(tm.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	raw, err := json.Marshal(tm.data)
	if err != nil {
		return err
	}
	return os.WriteFile(tm.filePath, raw, 0600)
}

func (tm *TOTPManager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(tm.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

func (tm *TOTPManager) decrypt(ciphertextHex string) (string, error) {
	data, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(tm.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func formatDuration(seconds int64) string {
	m := seconds / 60
	s := seconds % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
