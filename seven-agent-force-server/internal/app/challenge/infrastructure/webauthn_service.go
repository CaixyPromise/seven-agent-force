package infrastructure

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
)

type WebAuthnService struct {
	random *randominfra.Service
}

type clientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

func NewWebAuthnService(random *randominfra.Service) *WebAuthnService {
	return &WebAuthnService{random: random}
}

func (s *WebAuthnService) GenerateChallenge(ctx context.Context) (string, error) {
	if s == nil || s.random == nil {
		return "", fmt.Errorf("webauthn service is not configured")
	}
	return s.random.Nonce(ctx)
}

func (s *WebAuthnService) ValidateRegistrationClientData(raw, expectedChallenge string) bool {
	return validateClientData(raw, expectedChallenge, "webauthn.create")
}

func (s *WebAuthnService) ValidateAssertionClientData(raw, expectedChallenge string) bool {
	return validateClientData(raw, expectedChallenge, "webauthn.get")
}

func (s *WebAuthnService) ParseRegistrationAttestation(raw string) (*RegistrationAttestationData, error) {
	payload, ok := decodeMaybeBase64URL(raw)
	if !ok {
		payload = []byte(raw)
	}
	reader := cborReader{data: payload}
	length, err := reader.readMapLength()
	if err != nil {
		return nil, fmt.Errorf("parse attestation object: %w", err)
	}
	var format string
	var authData []byte
	for i := 0; i < length; i++ {
		key, err := reader.readText()
		if err != nil {
			return nil, fmt.Errorf("parse attestation key: %w", err)
		}
		switch key {
		case "fmt":
			format, err = reader.readText()
		case "authData":
			authData, err = reader.readBytes()
		case "attStmt":
			err = reader.skipValue()
		default:
			err = reader.skipValue()
		}
		if err != nil {
			return nil, fmt.Errorf("parse attestation %s: %w", key, err)
		}
	}
	if reader.pos != len(reader.data) {
		return nil, fmt.Errorf("attestation object has trailing data")
	}
	if strings.TrimSpace(format) == "" {
		return nil, fmt.Errorf("attestation format is empty")
	}
	return parseRegistrationAuthData(authData, format)
}

func (s *WebAuthnService) ValidateClientOrigin(clientDataJSON string, allowedOrigins []string) bool {
	return containsOrigin(allowedOrigins, clientDataJSON)
}

func (s *WebAuthnService) ValidateRpIDHash(authenticatorData string, rpID string) bool {
	data, ok := decodeMaybeBase64URL(authenticatorData)
	if !ok {
		data = []byte(authenticatorData)
	}
	if len(data) < 37 || strings.TrimSpace(rpID) == "" {
		return false
	}
	expected := sha256.Sum256([]byte(strings.TrimSpace(rpID)))
	return string(data[:32]) == string(expected[:])
}

func (s *WebAuthnService) ValidateRegistrationRpIDHash(attestation *RegistrationAttestationData, rpID string) bool {
	if attestation == nil || strings.TrimSpace(rpID) == "" {
		return false
	}
	rpIDHash, ok := decodeMaybeBase64URL(attestation.RPIDHash)
	if !ok || len(rpIDHash) != 32 {
		return false
	}
	expected := sha256.Sum256([]byte(strings.TrimSpace(rpID)))
	return string(rpIDHash) == string(expected[:])
}

func (s *WebAuthnService) ValidateUserPresence(authenticatorData string) bool {
	data, ok := decodeMaybeBase64URL(authenticatorData)
	if !ok {
		data = []byte(authenticatorData)
	}
	if len(data) < 37 {
		return false
	}
	flags := data[32]
	const (
		flagUserPresent  = byte(0x01)
		flagUserVerified = byte(0x04)
	)
	return flags&flagUserPresent != 0 && flags&flagUserVerified != 0
}

func (s *WebAuthnService) VerifyAssertionSignature(publicKeyCose, authenticatorData, clientDataJSON, signature string) bool {
	publicKey, err := parseCOSEPublicKey(publicKeyCose)
	if err != nil {
		return false
	}
	authenticatorBytes, ok := decodeMaybeBase64URL(authenticatorData)
	if !ok {
		authenticatorBytes = []byte(authenticatorData)
	}
	clientBytes, ok := decodeMaybeBase64URL(clientDataJSON)
	if !ok {
		clientBytes = []byte(clientDataJSON)
	}
	signatureBytes, ok := decodeMaybeBase64URL(signature)
	if !ok {
		signatureBytes = []byte(signature)
	}
	if len(authenticatorBytes) < 37 || len(clientBytes) == 0 || len(signatureBytes) == 0 {
		return false
	}
	clientHash := sha256.Sum256(clientBytes)
	signed := append(append([]byte(nil), authenticatorBytes...), clientHash[:]...)
	return publicKey.verify(signed, signatureBytes)
}

func (s *WebAuthnService) ValidatePublicKeyCose(publicKeyCose string) bool {
	_, err := parseCOSEPublicKey(publicKeyCose)
	return err == nil
}

func (s *WebAuthnService) ParseSignCount(authenticatorData string, fallback int64) int64 {
	data, ok := decodeMaybeBase64URL(authenticatorData)
	if !ok {
		data = []byte(authenticatorData)
	}
	if len(data) < 37 {
		if fallback < 0 {
			return 0
		}
		return fallback
	}
	value := binary.BigEndian.Uint32(data[33:37])
	return int64(value)
}

type RegistrationAttestationData struct {
	CredentialIdentifier string
	RPIDHash             string
	PublicKeyCose        string
	SignCount            int64
	AAGUID               string
	AttestationFormat    string
}

func parseRegistrationAuthData(authData []byte, format string) (*RegistrationAttestationData, error) {
	if len(authData) < 37 {
		return nil, fmt.Errorf("authenticator data is too short")
	}
	flags := authData[32]
	const (
		flagAttestedCredentialData = byte(0x40)
		flagExtensionData          = byte(0x80)
	)
	if flags&flagAttestedCredentialData == 0 {
		return nil, fmt.Errorf("authenticator data has no attested credential data")
	}
	signCount := binary.BigEndian.Uint32(authData[33:37])
	pos := 37
	if len(authData)-pos < 18 {
		return nil, fmt.Errorf("attested credential data is too short")
	}
	aaguid := append([]byte(nil), authData[pos:pos+16]...)
	pos += 16
	credentialIDLength := int(binary.BigEndian.Uint16(authData[pos : pos+2]))
	pos += 2
	if credentialIDLength <= 0 || len(authData)-pos < credentialIDLength {
		return nil, fmt.Errorf("credential id length exceeds authenticator data")
	}
	credentialID := append([]byte(nil), authData[pos:pos+credentialIDLength]...)
	pos += credentialIDLength
	if len(authData)-pos == 0 {
		return nil, fmt.Errorf("credential public key is missing")
	}
	keyStart := pos
	keyReader := cborReader{data: authData[pos:]}
	if err := keyReader.skipValue(); err != nil {
		return nil, fmt.Errorf("parse credential public key: %w", err)
	}
	publicKeyCoseBytes := append([]byte(nil), authData[keyStart:keyStart+keyReader.pos]...)
	pos += keyReader.pos
	if len(authData)-pos > 0 {
		if flags&flagExtensionData == 0 {
			return nil, fmt.Errorf("authenticator data has unexpected trailing bytes")
		}
		extensionReader := cborReader{data: authData[pos:]}
		if err := extensionReader.skipValue(); err != nil {
			return nil, fmt.Errorf("parse authenticator extensions: %w", err)
		}
		if extensionReader.pos != len(extensionReader.data) {
			return nil, fmt.Errorf("authenticator extensions have trailing data")
		}
	}
	publicKeyCose := base64.RawURLEncoding.EncodeToString(publicKeyCoseBytes)
	if _, err := parseCOSEPublicKey(publicKeyCose); err != nil {
		return nil, fmt.Errorf("parse credential public key: %w", err)
	}
	return &RegistrationAttestationData{
		CredentialIdentifier: base64.RawURLEncoding.EncodeToString(credentialID),
		RPIDHash:             base64.RawURLEncoding.EncodeToString(authData[:32]),
		PublicKeyCose:        publicKeyCose,
		SignCount:            int64(signCount),
		AAGUID:               formatAAGUID(aaguid),
		AttestationFormat:    strings.TrimSpace(format),
	}, nil
}

func formatAAGUID(value []byte) string {
	if len(value) != 16 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		value[0], value[1], value[2], value[3],
		value[4], value[5],
		value[6], value[7],
		value[8], value[9],
		value[10], value[11], value[12], value[13], value[14], value[15],
	)
}

func validateClientData(raw, expectedChallenge, expectedType string) bool {
	payload, ok := decodeMaybeBase64URL(raw)
	if !ok {
		payload = []byte(raw)
	}
	var data clientData
	if err := json.Unmarshal(payload, &data); err != nil {
		return false
	}
	if strings.TrimSpace(data.Type) != expectedType {
		return false
	}
	return normalizeChallenge(data.Challenge) == normalizeChallenge(expectedChallenge)
}

func decodeMaybeBase64URL(raw string) ([]byte, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return payload, true
	}
	payload, err = base64.StdEncoding.DecodeString(value)
	return payload, err == nil
}

func normalizeChallenge(value string) string {
	value = strings.TrimSpace(value)
	if payload, ok := decodeMaybeBase64URL(value); ok {
		return strings.TrimSpace(string(payload))
	}
	return value
}

func containsOrigin(allowedOrigins []string, rawClientData string) bool {
	if len(allowedOrigins) == 0 {
		return true
	}
	payload, ok := decodeMaybeBase64URL(rawClientData)
	if !ok {
		payload = []byte(rawClientData)
	}
	var data clientData
	if err := json.Unmarshal(payload, &data); err != nil {
		return false
	}
	for _, item := range allowedOrigins {
		if strings.TrimSpace(item) == strings.TrimSpace(data.Origin) {
			return true
		}
	}
	return false
}

type cosePublicKey struct {
	alg int
	key any
}

func (k cosePublicKey) verify(signed []byte, signature []byte) bool {
	switch publicKey := k.key.(type) {
	case *ecdsa.PublicKey:
		signedHash := sha256.Sum256(signed)
		return ecdsa.VerifyASN1(publicKey, signedHash[:], signature)
	case *rsa.PublicKey:
		signedHash := sha256.Sum256(signed)
		return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, signedHash[:], signature) == nil
	case ed25519.PublicKey:
		return ed25519.Verify(publicKey, signed, signature)
	default:
		return false
	}
}

func parseCOSEPublicKey(publicKeyCose string) (*cosePublicKey, error) {
	payload, ok := decodeMaybeBase64URL(publicKeyCose)
	if !ok {
		payload = []byte(publicKeyCose)
	}
	reader := cborReader{data: payload}
	length, err := reader.readMapLength()
	if err != nil {
		return nil, err
	}
	values := make(map[int]any, length)
	seen := make(map[int]struct{}, length)
	for i := 0; i < length; i++ {
		key, err := reader.readInt()
		if err != nil {
			return nil, err
		}
		if isRecognizedCOSEPublicKeyLabel(key) {
			if _, ok := seen[key]; ok {
				return nil, fmt.Errorf("duplicate cose key label")
			}
			seen[key] = struct{}{}
		}
		switch key {
		case 1, 3:
			value, err := reader.readInt()
			if err != nil {
				return nil, err
			}
			values[key] = value
		case -1:
			major, err := reader.peekMajorType()
			if err != nil {
				return nil, err
			}
			if major == 2 {
				value, err := reader.readBytes()
				if err != nil {
					return nil, err
				}
				values[key] = value
				continue
			}
			value, err := reader.readInt()
			if err != nil {
				return nil, err
			}
			values[key] = value
		case -2, -3:
			value, err := reader.readBytes()
			if err != nil {
				return nil, err
			}
			values[key] = value
		default:
			if err := reader.skipValue(); err != nil {
				return nil, err
			}
		}
	}
	if reader.pos != len(reader.data) {
		return nil, fmt.Errorf("cose key has trailing data")
	}
	kty, _ := values[1].(int)
	alg, _ := values[3].(int)
	crv, _ := values[-1].(int)
	nBytes, _ := values[-1].([]byte)
	xBytes, _ := values[-2].([]byte)
	yBytes, _ := values[-3].([]byte)
	switch {
	case kty == 2 && alg == -7 && crv == 1:
		return parseCOSEEC2P256PublicKey(xBytes, yBytes)
	case kty == 3 && alg == -257:
		return parseCOSERS256PublicKey(nBytes, xBytes)
	case kty == 1 && alg == -8 && crv == 6:
		return parseCOSEEd25519PublicKey(xBytes)
	default:
		return nil, fmt.Errorf("unsupported cose key")
	}
}

func parseCOSEEC2P256PublicKey(xBytes, yBytes []byte) (*cosePublicKey, error) {
	if len(xBytes) != 32 || len(yBytes) != 32 {
		return nil, fmt.Errorf("unsupported cose key")
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("invalid cose key coordinates")
	}
	return &cosePublicKey{alg: -7, key: &ecdsa.PublicKey{Curve: curve, X: x, Y: y}}, nil
}

func parseCOSERS256PublicKey(nBytes, eBytes []byte) (*cosePublicKey, error) {
	n := new(big.Int).SetBytes(nBytes)
	if n.Sign() <= 0 || n.BitLen() < 2048 || n.Bit(0) != 1 {
		return nil, fmt.Errorf("invalid rsa cose key")
	}
	if !bytes.Equal(eBytes, []byte{0x01, 0x00, 0x01}) {
		return nil, fmt.Errorf("invalid rsa cose key")
	}
	return &cosePublicKey{alg: -257, key: &rsa.PublicKey{N: n, E: 65537}}, nil
}

func parseCOSEEd25519PublicKey(xBytes []byte) (*cosePublicKey, error) {
	if len(xBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 cose key")
	}
	return &cosePublicKey{alg: -8, key: ed25519.PublicKey(append([]byte(nil), xBytes...))}, nil
}

func isRecognizedCOSEPublicKeyLabel(key int) bool {
	switch key {
	case 1, 3, -1, -2, -3:
		return true
	default:
		return false
	}
}

type cborReader struct {
	data []byte
	pos  int
}

func (r *cborReader) peekMajorType() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("unexpected cbor end")
	}
	return r.data[r.pos] >> 5, nil
}

func (r *cborReader) readMapLength() (int, error) {
	major, value, err := r.readHeader()
	if err != nil {
		return 0, err
	}
	if major != 5 {
		return 0, fmt.Errorf("cbor value is not a map")
	}
	return int(value), nil
}

func (r *cborReader) readInt() (int, error) {
	major, value, err := r.readHeader()
	if err != nil {
		return 0, err
	}
	switch major {
	case 0:
		return int(value), nil
	case 1:
		return -1 - int(value), nil
	default:
		return 0, fmt.Errorf("cbor value is not an int")
	}
}

func (r *cborReader) readBytes() ([]byte, error) {
	major, value, err := r.readHeader()
	if err != nil {
		return nil, err
	}
	if major != 2 {
		return nil, fmt.Errorf("cbor value is not bytes")
	}
	if value > uint64(len(r.data)-r.pos) {
		return nil, fmt.Errorf("cbor bytes length exceeds payload")
	}
	result := append([]byte(nil), r.data[r.pos:r.pos+int(value)]...)
	r.pos += int(value)
	return result, nil
}

func (r *cborReader) readText() (string, error) {
	major, value, err := r.readHeader()
	if err != nil {
		return "", err
	}
	if major != 3 {
		return "", fmt.Errorf("cbor value is not text")
	}
	if value > uint64(len(r.data)-r.pos) {
		return "", fmt.Errorf("cbor text length exceeds payload")
	}
	result := string(r.data[r.pos : r.pos+int(value)])
	r.pos += int(value)
	return result, nil
}

func (r *cborReader) skipValue() error {
	major, value, err := r.readHeader()
	if err != nil {
		return err
	}
	switch major {
	case 0, 1, 7:
		return nil
	case 2, 3:
		if value > uint64(len(r.data)-r.pos) {
			return fmt.Errorf("cbor skip length exceeds payload")
		}
		r.pos += int(value)
		return nil
	case 4:
		for i := uint64(0); i < value; i++ {
			if err := r.skipValue(); err != nil {
				return err
			}
		}
		return nil
	case 5:
		for i := uint64(0); i < value; i++ {
			if err := r.skipValue(); err != nil {
				return err
			}
			if err := r.skipValue(); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported cbor major type %d", major)
	}
}

func (r *cborReader) readHeader() (byte, uint64, error) {
	if r.pos >= len(r.data) {
		return 0, 0, fmt.Errorf("unexpected cbor end")
	}
	header := r.data[r.pos]
	r.pos++
	major := header >> 5
	additional := header & 0x1f
	switch {
	case additional < 24:
		return major, uint64(additional), nil
	case additional == 24:
		if r.pos >= len(r.data) {
			return 0, 0, fmt.Errorf("unexpected cbor uint8 end")
		}
		value := r.data[r.pos]
		r.pos++
		return major, uint64(value), nil
	case additional == 25:
		if len(r.data)-r.pos < 2 {
			return 0, 0, fmt.Errorf("unexpected cbor uint16 end")
		}
		value := binary.BigEndian.Uint16(r.data[r.pos : r.pos+2])
		r.pos += 2
		return major, uint64(value), nil
	case additional == 26:
		if len(r.data)-r.pos < 4 {
			return 0, 0, fmt.Errorf("unexpected cbor uint32 end")
		}
		value := binary.BigEndian.Uint32(r.data[r.pos : r.pos+4])
		r.pos += 4
		return major, uint64(value), nil
	default:
		return 0, 0, fmt.Errorf("unsupported cbor additional info %d", additional)
	}
}
