package infrastructure

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestWebAuthnServiceParsesRegistrationAttestationObject(t *testing.T) {
	service := NewWebAuthnService(nil)
	publicKeyCose := mustCOSEEC2PublicKey(t)
	aaguid := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	attestationObject := mustAttestationObject(t, "none", "credential-1", publicKeyCose, 7, aaguid)

	parsed, err := service.ParseRegistrationAttestation(attestationObject)
	if err != nil {
		t.Fatalf("parse attestation object: %v", err)
	}
	if parsed == nil {
		t.Fatal("expected parsed attestation data")
	}
	if parsed.AttestationFormat != "none" {
		t.Fatalf("unexpected attestation format: %q", parsed.AttestationFormat)
	}
	if parsed.CredentialIdentifier != base64.RawURLEncoding.EncodeToString([]byte("credential-1")) {
		t.Fatalf("unexpected credential id: %q", parsed.CredentialIdentifier)
	}
	if parsed.PublicKeyCose != publicKeyCose {
		t.Fatalf("unexpected publicKeyCose: %q", parsed.PublicKeyCose)
	}
	if parsed.SignCount != 7 {
		t.Fatalf("unexpected sign count: %d", parsed.SignCount)
	}
	if parsed.AAGUID != "01020304-0506-0708-090a-0b0c0d0e0f10" {
		t.Fatalf("unexpected aaguid: %q", parsed.AAGUID)
	}
}

func TestWebAuthnServiceRejectsRegistrationAttestationWithoutAttestedCredentialData(t *testing.T) {
	service := NewWebAuthnService(nil)
	authData := make([]byte, 37)
	authData[32] = 0x01
	attestationObject := encodeCBORMap(map[string][]byte{
		"fmt":      encodeCBORText("none"),
		"authData": encodeCBORBytes(authData),
		"attStmt":  []byte{0xa0},
	})

	if parsed, err := service.ParseRegistrationAttestation(base64.RawURLEncoding.EncodeToString(attestationObject)); err == nil || parsed != nil {
		t.Fatalf("expected attestation without credential data to fail, parsed=%+v err=%v", parsed, err)
	}
}

func TestWebAuthnServiceRejectsPublicKeyCoseWithTrailingGarbage(t *testing.T) {
	service := NewWebAuthnService(nil)
	valid := mustCOSEEC2PublicKey(t)
	if !service.ValidatePublicKeyCose(valid) {
		t.Fatal("test fixture must be a valid COSE public key")
	}
	garbage := base64.RawURLEncoding.EncodeToString(append(mustDecodeBase64URL(t, valid), []byte("garbage")...))

	if service.ValidatePublicKeyCose(garbage) {
		t.Fatal("expected COSE public key with trailing garbage to be rejected")
	}
}

func TestWebAuthnServiceVerifiesRS256AssertionSignature(t *testing.T) {
	service := NewWebAuthnService(nil)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	publicKeyCose := encodeCOSERS256PublicKey(t, &privateKey.PublicKey)
	if !service.ValidatePublicKeyCose(publicKeyCose) {
		t.Fatal("expected RS256 COSE public key to be accepted")
	}
	attestationObject := mustAttestationObject(t, "none", "credential-rsa", publicKeyCose, 1, make([]byte, 16))
	if _, err := service.ParseRegistrationAttestation(attestationObject); err != nil {
		t.Fatalf("expected RS256 attestation public key to parse: %v", err)
	}
	authenticatorData, clientDataJSON := signedWebAuthnAssertionInput(t)
	signed := webAuthnSignedBytes(t, authenticatorData, clientDataJSON)
	digest := sha256.Sum256(signed)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign rsa assertion: %v", err)
	}

	if !service.VerifyAssertionSignature(
		publicKeyCose,
		base64.RawURLEncoding.EncodeToString(authenticatorData),
		base64.RawURLEncoding.EncodeToString(clientDataJSON),
		base64.RawURLEncoding.EncodeToString(signature),
	) {
		t.Fatal("expected RS256 assertion signature to verify")
	}
}

func TestWebAuthnServiceVerifiesEdDSAAssertionSignature(t *testing.T) {
	service := NewWebAuthnService(nil)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	publicKeyCose := encodeCOSEEd25519PublicKey(t, publicKey)
	if !service.ValidatePublicKeyCose(publicKeyCose) {
		t.Fatal("expected Ed25519 COSE public key to be accepted")
	}
	attestationObject := mustAttestationObject(t, "none", "credential-ed25519", publicKeyCose, 1, make([]byte, 16))
	if _, err := service.ParseRegistrationAttestation(attestationObject); err != nil {
		t.Fatalf("expected Ed25519 attestation public key to parse: %v", err)
	}
	authenticatorData, clientDataJSON := signedWebAuthnAssertionInput(t)
	signature := ed25519.Sign(privateKey, webAuthnSignedBytes(t, authenticatorData, clientDataJSON))

	if !service.VerifyAssertionSignature(
		publicKeyCose,
		base64.RawURLEncoding.EncodeToString(authenticatorData),
		base64.RawURLEncoding.EncodeToString(clientDataJSON),
		base64.RawURLEncoding.EncodeToString(signature),
	) {
		t.Fatal("expected Ed25519 assertion signature to verify")
	}
}

func TestWebAuthnServiceRejectsWeakOrMalformedCOSEPublicKeys(t *testing.T) {
	service := NewWebAuthnService(nil)
	validModulus := append([]byte{0x80}, make([]byte, 255)...)
	validModulus[len(validModulus)-1] = 0x01
	evenModulus := append([]byte(nil), validModulus...)
	evenModulus[len(evenModulus)-1] = 0x00
	tests := []struct {
		name    string
		modulus []byte
		exp     []byte
	}{
		{
			name:    "sub-2048-bit modulus",
			modulus: append([]byte{0x80}, make([]byte, 127)...),
			exp:     []byte{0x01, 0x00, 0x01},
		},
		{
			name:    "zero modulus",
			modulus: []byte{0x00},
			exp:     []byte{0x01, 0x00, 0x01},
		},
		{
			name:    "even modulus",
			modulus: evenModulus,
			exp:     []byte{0x01, 0x00, 0x01},
		},
		{
			name:    "exponent one",
			modulus: validModulus,
			exp:     []byte{0x01},
		},
		{
			name:    "exponent three",
			modulus: validModulus,
			exp:     []byte{0x03},
		},
		{
			name:    "even exponent",
			modulus: validModulus,
			exp:     []byte{0x02},
		},
		{
			name:    "oversized exponent encoding",
			modulus: validModulus,
			exp:     []byte{0x00, 0x00, 0x01, 0x00, 0x01},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weakRSA := encodeCOSERS256PublicKeyRaw(tt.modulus, tt.exp)
			if service.ValidatePublicKeyCose(weakRSA) {
				t.Fatalf("expected malformed RS256 COSE public key to be rejected")
			}
		})
	}
	malformedEd25519 := base64.RawURLEncoding.EncodeToString(encodeCBORIntMap(map[int][]byte{
		1:  encodeCBORInt(1),
		3:  encodeCBORInt(-8),
		-1: encodeCBORInt(6),
		-2: encodeCBORBytes(make([]byte, ed25519.PublicKeySize-1)),
	}))
	if service.ValidatePublicKeyCose(malformedEd25519) {
		t.Fatal("expected Ed25519 COSE public key with wrong key length to be rejected")
	}
}

func TestWebAuthnServiceRejectsDuplicateCOSELabels(t *testing.T) {
	service := NewWebAuthnService(nil)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	x := privateKey.PublicKey.X.Bytes()
	y := privateKey.PublicKey.Y.Bytes()
	paddedX := append(make([]byte, 32-len(x)), x...)
	paddedY := append(make([]byte, 32-len(y)), y...)
	basePairs := []cborIntPair{
		{key: 1, value: encodeCBORInt(2)},
		{key: 3, value: encodeCBORInt(-7)},
		{key: -1, value: encodeCBORInt(1)},
		{key: -2, value: encodeCBORBytes(paddedX)},
		{key: -3, value: encodeCBORBytes(paddedY)},
	}
	duplicateCases := []struct {
		name string
		pair cborIntPair
	}{
		{name: "kty", pair: basePairs[0]},
		{name: "alg", pair: basePairs[1]},
		{name: "crv", pair: basePairs[2]},
		{name: "x", pair: basePairs[3]},
		{name: "y", pair: basePairs[4]},
	}
	for _, tt := range duplicateCases {
		t.Run(tt.name, func(t *testing.T) {
			pairs := append([]cborIntPair{}, basePairs...)
			pairs = append(pairs, tt.pair)
			duplicateCOSE := base64.RawURLEncoding.EncodeToString(encodeCBORIntPairs(pairs...))
			if service.ValidatePublicKeyCose(duplicateCOSE) {
				t.Fatalf("expected duplicate COSE %s label to be rejected", tt.name)
			}
		})
	}
}

func mustDecodeBase64URL(t *testing.T, value string) []byte {
	t.Helper()
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64url: %v", err)
	}
	return payload
}

func mustCOSEEC2PublicKey(t *testing.T) string {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	x := privateKey.PublicKey.X.Bytes()
	y := privateKey.PublicKey.Y.Bytes()
	paddedX := append(make([]byte, 32-len(x)), x...)
	paddedY := append(make([]byte, 32-len(y)), y...)
	cose := []byte{
		0xa5,
		0x01, 0x02,
		0x03, 0x26,
		0x20, 0x01,
		0x21, 0x58, 0x20,
	}
	cose = append(cose, paddedX...)
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, paddedY...)
	return base64.RawURLEncoding.EncodeToString(cose)
}

func encodeCOSERS256PublicKey(t *testing.T, publicKey *rsa.PublicKey) string {
	t.Helper()
	return encodeCOSERS256PublicKeyRaw(publicKey.N.Bytes(), bigEndianInt(publicKey.E))
}

func encodeCOSERS256PublicKeyRaw(modulus, exponent []byte) string {
	return base64.RawURLEncoding.EncodeToString(encodeCBORIntMap(map[int][]byte{
		1:  encodeCBORInt(3),
		3:  encodeCBORInt(-257),
		-1: encodeCBORBytes(modulus),
		-2: encodeCBORBytes(exponent),
	}))
}

func encodeCOSEEd25519PublicKey(t *testing.T, publicKey ed25519.PublicKey) string {
	t.Helper()
	if len(publicKey) != ed25519.PublicKeySize {
		t.Fatalf("unexpected ed25519 public key length: %d", len(publicKey))
	}
	cose := encodeCBORIntMap(map[int][]byte{
		1:  encodeCBORInt(1),
		3:  encodeCBORInt(-8),
		-1: encodeCBORInt(6),
		-2: encodeCBORBytes(publicKey),
	})
	return base64.RawURLEncoding.EncodeToString(cose)
}

func mustAttestationObject(t *testing.T, format, credentialID, publicKeyCose string, signCount uint32, aaguid []byte) string {
	t.Helper()
	authData := make([]byte, 37)
	authData[32] = 0x41
	binary.BigEndian.PutUint32(authData[33:37], signCount)
	authData = append(authData, aaguid...)
	credentialIDBytes := []byte(credentialID)
	authData = append(authData, byte(len(credentialIDBytes)>>8), byte(len(credentialIDBytes)))
	authData = append(authData, credentialIDBytes...)
	authData = append(authData, mustDecodeBase64URL(t, publicKeyCose)...)
	object := encodeCBORMap(map[string][]byte{
		"fmt":      encodeCBORText(format),
		"authData": encodeCBORBytes(authData),
		"attStmt":  []byte{0xa0},
	})
	return base64.RawURLEncoding.EncodeToString(object)
}

func signedWebAuthnAssertionInput(t *testing.T) ([]byte, []byte) {
	t.Helper()
	rpHash := sha256.Sum256([]byte("example.com"))
	authenticatorData := make([]byte, 37)
	copy(authenticatorData[:32], rpHash[:])
	authenticatorData[32] = 0x05
	binary.BigEndian.PutUint32(authenticatorData[33:37], 2)
	clientDataJSON, err := json.Marshal(map[string]any{
		"type":      "webauthn.get",
		"challenge": "challenge-1",
		"origin":    "https://example.com",
	})
	if err != nil {
		t.Fatalf("marshal client data: %v", err)
	}
	return authenticatorData, clientDataJSON
}

func webAuthnSignedBytes(t *testing.T, authenticatorData, clientDataJSON []byte) []byte {
	t.Helper()
	clientHash := sha256.Sum256(clientDataJSON)
	return append(append([]byte(nil), authenticatorData...), clientHash[:]...)
}

func encodeCBORMap(values map[string][]byte) []byte {
	result := []byte{0xa0 | byte(len(values))}
	for _, key := range []string{"fmt", "authData", "attStmt"} {
		value, ok := values[key]
		if !ok {
			continue
		}
		result = append(result, encodeCBORText(key)...)
		result = append(result, value...)
	}
	return result
}

func encodeCBORIntMap(values map[int][]byte) []byte {
	keys := []int{1, 3, -1, -2, -3}
	result := encodeCBORHeader(5, len(values))
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		result = append(result, encodeCBORInt(key)...)
		result = append(result, value...)
	}
	return result
}

type cborIntPair struct {
	key   int
	value []byte
}

func encodeCBORIntPairs(values ...cborIntPair) []byte {
	result := encodeCBORHeader(5, len(values))
	for _, item := range values {
		result = append(result, encodeCBORInt(item.key)...)
		result = append(result, item.value...)
	}
	return result
}

func encodeCBORInt(value int) []byte {
	if value >= 0 {
		return encodeCBORHeader(0, value)
	}
	return encodeCBORHeader(1, -1-value)
}

func encodeCBORText(value string) []byte {
	result := encodeCBORHeader(3, len(value))
	return append(result, []byte(value)...)
}

func encodeCBORBytes(value []byte) []byte {
	result := encodeCBORHeader(2, len(value))
	return append(result, value...)
}

func encodeCBORHeader(major byte, length int) []byte {
	switch {
	case length < 24:
		return []byte{major<<5 | byte(length)}
	case length <= 0xff:
		return []byte{major<<5 | 24, byte(length)}
	case length <= 0xffff:
		return []byte{major<<5 | 25, byte(length >> 8), byte(length)}
	default:
		return []byte{major<<5 | 26, byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	}
}

func bigEndianInt(value int) []byte {
	if value == 0 {
		return []byte{0}
	}
	var reversed []byte
	for value > 0 {
		reversed = append(reversed, byte(value))
		value >>= 8
	}
	result := make([]byte, len(reversed))
	for i := range reversed {
		result[i] = reversed[len(reversed)-1-i]
	}
	return result
}
