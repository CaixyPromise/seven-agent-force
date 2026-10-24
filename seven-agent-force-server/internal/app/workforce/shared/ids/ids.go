package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func New(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(fmt.Errorf("generate id: %w", err))
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}
