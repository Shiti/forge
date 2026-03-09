package idgen

import (
	"math/big"
	"strings"

	"github.com/google/uuid"
)

const (
	shortUUIDAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	shortUUIDLength   = 22
)

var shortUUIDBase = big.NewInt(int64(len(shortUUIDAlphabet)))

// NewShortUUID generates a shortuuid-compatible identifier from a v4 UUID.
func NewShortUUID() string {
	return EncodeUUID(uuid.New())
}

// EncodeUUID converts a UUID into shortuuid alphabet form.
func EncodeUUID(u uuid.UUID) string {
	num := new(big.Int).SetBytes(u[:])
	zero := big.NewInt(0)

	chars := make([]byte, 0, shortUUIDLength)
	mod := new(big.Int)
	for num.Cmp(zero) > 0 {
		num.DivMod(num, shortUUIDBase, mod)
		chars = append(chars, shortUUIDAlphabet[mod.Int64()])
	}

	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}

	if len(chars) < shortUUIDLength {
		pad := strings.Repeat(string(shortUUIDAlphabet[0]), shortUUIDLength-len(chars))
		return pad + string(chars)
	}
	return string(chars)
}
