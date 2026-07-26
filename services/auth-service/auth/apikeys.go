package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidAPIKey = errors.New("invalid or non-existent API key")
	ErrRevokedAPIKey = errors.New("API key has been revoked")
)

type APIKeyRecord struct {
	ID        string
	Name      string
	KeyHash   string // SHA-256 hash of raw key (NEVER raw key!)
	Role      string
	Revoked   bool
	CreatedAt time.Time
}

type APIKeyStore struct {
	mu     sync.RWMutex
	keys   map[string]*APIKeyRecord // KeyHash -> APIKeyRecord
	hashes map[string]string        // Key ID -> KeyHash
}

var globalKeyStore = NewAPIKeyStore()

func NewAPIKeyStore() *APIKeyStore {
	return &APIKeyStore{
		keys:   make(map[string]*APIKeyRecord),
		hashes: make(map[string]string),
	}
}

func GetGlobalKeyStore() *APIKeyStore {
	return globalKeyStore
}

// GenerateRawKey produces a secure random long-lived API key string.
func GenerateRawKey() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("nb_ak_%s", hex.EncodeToString(bytes)), nil
}

// HashKey computes the SHA-256 hash of a raw API key.
func HashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// CreateAPIKey generates a new key, hashes it, stores the hash, and returns the raw key to the caller once.
func (s *APIKeyStore) CreateAPIKey(name string, role string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rawKey, err := GenerateRawKey()
	if err != nil {
		return "", "", err
	}

	keyHash := HashKey(rawKey)
	keyID := fmt.Sprintf("key-%d", time.Now().UnixNano())

	rec := &APIKeyRecord{
		ID:        keyID,
		Name:      name,
		KeyHash:   keyHash,
		Role:      role,
		Revoked:   false,
		CreatedAt: time.Now(),
	}

	s.keys[keyHash] = rec
	s.hashes[keyID] = keyHash

	return keyID, rawKey, nil
}

// VerifyAPIKey hashes the presented raw key and checks if it exists and is active.
func (s *APIKeyStore) VerifyAPIKey(rawKey string) (*APIKeyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keyHash := HashKey(rawKey)
	rec, exists := s.keys[keyHash]
	if !exists {
		return nil, ErrInvalidAPIKey
	}
	if rec.Revoked {
		return nil, ErrRevokedAPIKey
	}

	return rec, nil
}

// RevokeAPIKey immediately revokes an API key by its Key ID or Hash.
func (s *APIKeyStore) RevokeAPIKey(keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	keyHash, exists := s.hashes[keyID]
	if !exists {
		keyHash = keyID // allow direct hash lookup
	}

	rec, exists := s.keys[keyHash]
	if !exists {
		return ErrInvalidAPIKey
	}

	rec.Revoked = true
	return nil
}
