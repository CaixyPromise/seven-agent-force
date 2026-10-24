package aes

import (
	"bytes"
	stdaes "crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

type Mode string

const (
	ModeCBC Mode = "CBC"
	ModeGCM Mode = "GCM"
	ModeECB Mode = "ECB"
	ModeCFB Mode = "CFB"
	ModeOFB Mode = "OFB"
	ModeCTR Mode = "CTR"
)

type OpMode int

const (
	DecryptMode OpMode = iota
	EncryptMode
)

type CipherSpec struct {
	Mode   Mode
	KeyB64 string
	IVB64  string
}

type Cipher interface {
	DoFinal(input []byte) ([]byte, error)
}

type CipherFactory interface {
	NewCipher(mode Mode, keyB64, ivB64 string, opMode OpMode) (Cipher, error)
	Generate(mode Mode) (map[string]string, error)
}

type factory struct{}

func NewFactory() CipherFactory {
	return factory{}
}

func Generate(mode Mode) (map[string]string, error) {
	return NewFactory().Generate(mode)
}

func NewCipher(mode Mode, keyB64, ivB64 string, opMode OpMode) (Cipher, error) {
	return NewFactory().NewCipher(mode, keyB64, ivB64, opMode)
}

func Encrypt(plain string, spec CipherSpec) (string, error) {
	c, err := NewCipher(spec.Mode, spec.KeyB64, spec.IVB64, EncryptMode)
	if err != nil {
		return "", err
	}
	encrypted, err := c.DoFinal([]byte(plain))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func Decrypt(ciphertext string, spec CipherSpec) (string, error) {
	c, err := NewCipher(spec.Mode, spec.KeyB64, spec.IVB64, DecryptMode)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext base64: %w", err)
	}
	plain, err := c.DoFinal(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (factory) Generate(mode Mode) (map[string]string, error) {
	mode, err := normalizeMode(mode)
	if err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate aes key: %w", err)
	}
	result := map[string]string{
		"key": base64.StdEncoding.EncodeToString(key),
	}
	if ivSize := mode.ivSize(); ivSize > 0 {
		iv := make([]byte, ivSize)
		if _, err := rand.Read(iv); err != nil {
			return nil, fmt.Errorf("generate aes iv: %w", err)
		}
		result["iv"] = base64.StdEncoding.EncodeToString(iv)
	}
	return result, nil
}

func (factory) NewCipher(mode Mode, keyB64, ivB64 string, opMode OpMode) (Cipher, error) {
	mode, err := normalizeMode(mode)
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(keyB64)
	if err != nil {
		return nil, err
	}
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	iv, err := decodeIV(mode, ivB64)
	if err != nil {
		return nil, err
	}
	return newModeCipher(mode, block, iv, opMode)
}

func normalizeMode(mode Mode) (Mode, error) {
	switch mode {
	case ModeCBC, ModeGCM, ModeECB, ModeCFB, ModeOFB, ModeCTR:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported aes mode: %s", mode)
	}
}

func (m Mode) ivSize() int {
	switch m {
	case ModeGCM:
		return 12
	case ModeECB:
		return 0
	default:
		return stdaes.BlockSize
	}
}

func decodeKey(keyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("decode aes key: %w", err)
	}
	switch len(key) {
	case 16, 24, 32:
		return key, nil
	default:
		return nil, fmt.Errorf("invalid aes key length: %d", len(key))
	}
}

func decodeIV(mode Mode, ivB64 string) ([]byte, error) {
	if mode.ivSize() == 0 {
		return nil, nil
	}
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return nil, fmt.Errorf("decode aes iv: %w", err)
	}
	if len(iv) != mode.ivSize() {
		return nil, fmt.Errorf("invalid aes iv length for %s: got %d want %d", mode, len(iv), mode.ivSize())
	}
	return iv, nil
}

func newModeCipher(mode Mode, block cipher.Block, iv []byte, opMode OpMode) (Cipher, error) {
	switch mode {
	case ModeECB:
		return ecbCipher{block: block, opMode: opMode}, nil
	case ModeCBC:
		return cbcCipher{block: block, iv: iv, opMode: opMode}, nil
	case ModeGCM:
		gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
		if err != nil {
			return nil, fmt.Errorf("new gcm: %w", err)
		}
		return gcmCipher{aead: gcm, iv: iv, opMode: opMode}, nil
	case ModeCFB:
		return cfbCipher{block: block, iv: iv, opMode: opMode}, nil
	case ModeOFB:
		return ofbCipher{block: block, iv: iv}, nil
	case ModeCTR:
		return ctrCipher{block: block, iv: iv}, nil
	default:
		return nil, fmt.Errorf("unsupported aes mode: %s", mode)
	}
}

type ecbCipher struct {
	block  cipher.Block
	opMode OpMode
}

func (c ecbCipher) DoFinal(input []byte) ([]byte, error) {
	blockSize := c.block.BlockSize()
	if c.opMode == EncryptMode {
		input = pkcs7Pad(input, blockSize)
	} else if len(input)%blockSize != 0 {
		return nil, fmt.Errorf("ecb ciphertext is not a multiple of block size")
	}
	output := make([]byte, len(input))
	for offset := 0; offset < len(input); offset += blockSize {
		if c.opMode == EncryptMode {
			c.block.Encrypt(output[offset:offset+blockSize], input[offset:offset+blockSize])
		} else {
			c.block.Decrypt(output[offset:offset+blockSize], input[offset:offset+blockSize])
		}
	}
	if c.opMode == DecryptMode {
		return pkcs7Unpad(output, blockSize)
	}
	return output, nil
}

type cbcCipher struct {
	block  cipher.Block
	iv     []byte
	opMode OpMode
}

func (c cbcCipher) DoFinal(input []byte) ([]byte, error) {
	blockSize := c.block.BlockSize()
	if c.opMode == EncryptMode {
		input = pkcs7Pad(input, blockSize)
		output := make([]byte, len(input))
		cipher.NewCBCEncrypter(c.block, c.iv).CryptBlocks(output, input)
		return output, nil
	}
	if len(input)%blockSize != 0 {
		return nil, fmt.Errorf("cbc ciphertext is not a multiple of block size")
	}
	output := make([]byte, len(input))
	cipher.NewCBCDecrypter(c.block, c.iv).CryptBlocks(output, input)
	return pkcs7Unpad(output, blockSize)
}

type gcmCipher struct {
	aead   cipher.AEAD
	iv     []byte
	opMode OpMode
}

func (c gcmCipher) DoFinal(input []byte) ([]byte, error) {
	if c.opMode == EncryptMode {
		return c.aead.Seal(nil, c.iv, input, nil), nil
	}
	output, err := c.aead.Open(nil, c.iv, input, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm decrypt: %w", err)
	}
	return output, nil
}

type cfbCipher struct {
	block  cipher.Block
	iv     []byte
	opMode OpMode
}

func (c cfbCipher) DoFinal(input []byte) ([]byte, error) {
	output := make([]byte, len(input))
	if c.opMode == EncryptMode {
		cipher.NewCFBEncrypter(c.block, c.iv).XORKeyStream(output, input)
		return output, nil
	}
	cipher.NewCFBDecrypter(c.block, c.iv).XORKeyStream(output, input)
	return output, nil
}

type ofbCipher struct {
	block cipher.Block
	iv    []byte
}

func (c ofbCipher) DoFinal(input []byte) ([]byte, error) {
	output := make([]byte, len(input))
	cipher.NewOFB(c.block, c.iv).XORKeyStream(output, input)
	return output, nil
}

type ctrCipher struct {
	block cipher.Block
	iv    []byte
}

func (c ctrCipher) DoFinal(input []byte) ([]byte, error) {
	output := make([]byte, len(input))
	cipher.NewCTR(c.block, c.iv).XORKeyStream(output, input)
	return output, nil
}

func pkcs7Pad(input []byte, blockSize int) []byte {
	padding := blockSize - (len(input) % blockSize)
	return append(input, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(input []byte, blockSize int) ([]byte, error) {
	if len(input) == 0 || len(input)%blockSize != 0 {
		return nil, fmt.Errorf("invalid pkcs7 payload length")
	}
	padding := int(input[len(input)-1])
	if padding == 0 || padding > blockSize || padding > len(input) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	for i := len(input) - padding; i < len(input); i++ {
		if int(input[i]) != padding {
			return nil, fmt.Errorf("invalid pkcs7 padding bytes")
		}
	}
	return input[:len(input)-padding], nil
}
