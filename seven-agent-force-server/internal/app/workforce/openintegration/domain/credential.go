package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

var ErrInvalidCredential = errors.New("invalid integration credential")

func GenerateAPIKey() (credentialID, token, digest, hint string, err error) {
	idRaw := make([]byte, 12)
	secretRaw := make([]byte, 32)
	if _, err = rand.Read(idRaw); err != nil {
		return "", "", "", "", err
	}
	if _, err = rand.Read(secretRaw); err != nil {
		return "", "", "", "", err
	}
	credentialID = hex.EncodeToString(idRaw)
	secret := base64.RawURLEncoding.EncodeToString(secretRaw)
	token = "swk_" + credentialID + "." + secret
	digest = Digest(token)
	hint = token[len(token)-8:]
	return
}

func ParseAPIKey(token string) (string, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "swk_") {
		return "", ErrInvalidCredential
	}
	credentialID, secret, ok := strings.Cut(strings.TrimPrefix(token, "swk_"), ".")
	if !ok || len(credentialID) != 24 || len(secret) != 43 {
		return "", ErrInvalidCredential
	}
	if _, err := hex.DecodeString(credentialID); err != nil {
		return "", ErrInvalidCredential
	}
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(raw) != 32 {
		return "", ErrInvalidCredential
	}
	return credentialID, nil
}

func GenerateSigningSecret() (token, hint string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	token = "whsec_" + base64.RawURLEncoding.EncodeToString(raw)
	hint = token[len(token)-8:]
	return
}

func Digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func DigestMatches(value, expectedHex string) bool {
	actual := sha256.Sum256([]byte(value))
	expected, err := hex.DecodeString(expectedHex)
	if err != nil || len(expected) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(actual[:], expected) == 1
}
