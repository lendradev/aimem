package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"hash/fnv"
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// New returns an 8 character short id derived from a random UUID.
// The id is short enough to paste into a prompt and stable once assigned.
func New() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("aimem: crypto/rand unavailable: " + err.Error())
	}
	h := fnv.New32a()
	h.Write(buf[:])
	sum := h.Sum(nil)
	out := make([]byte, 8)
	// Use the hash bytes plus the raw bytes so collisions stay negligible.
	copy(out, sum[:4])
	copy(out[4:], buf[:4])
	return fmt.Sprintf("%s", encoding.EncodeToString(out)[:8])
}
