package spotify

import (
	"fmt"
	"math/big"
	"strings"
)

// base62 is how Spotify writes an ID in a URI: the same 128 bits in base 62,
// digits then lowercase then uppercase. The order matters — with A-Z first
// every ID decodes to a different set of bits, which Spotify answers 404
// for, and nothing says why.
const base62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GID returns id's 128-bit form in hex, as the spclient paths name it:
// metadata/4/track/{gid}, manifests/v9/json/sources/{gid}. id is a URI's
// part (`0VjIjW4GlUZAMYd2vXMi3b`) or the hex GID itself, which comes back
// unchanged; a `spotify:kind:id` URI is taken apart first.
func GID(id string) (string, error) {
	if _, _, ok := strings.Cut(id, ":"); ok {
		parts := strings.Split(id, ":")
		if len(parts) < 3 || parts[1] == "" {
			return "", fmt.Errorf("spotify: not an ID: %q", id)
		}
		id = parts[2]
	}
	switch {
	case isHex(id) && len(id) == 32:
		return strings.ToLower(id), nil
	case len(id) == 22:
		n := new(big.Int)
		for _, c := range id {
			d := strings.IndexRune(base62, c)
			if d < 0 {
				return "", fmt.Errorf("spotify: not base 62: %q", id)
			}
			n.Mul(n, big.NewInt(62)).Add(n, big.NewInt(int64(d)))
		}
		if n.BitLen() > 128 {
			return "", fmt.Errorf("spotify: not an ID, over 128 bits: %q", id)
		}
		return fmt.Sprintf("%032x", n), nil
	default:
		return "", fmt.Errorf("spotify: not an ID: %q", id)
	}
}

// Base62 returns the ID a URI writes for gid, the hex form paths take.
func Base62(gid string) (string, error) {
	if !isHex(gid) || len(gid) != 32 {
		return "", fmt.Errorf("spotify: not a GID: %q", gid)
	}
	n, ok := new(big.Int).SetString(gid, 16)
	if !ok {
		return "", fmt.Errorf("spotify: not a GID: %q", gid)
	}
	out := make([]byte, 22)
	for i := len(out) - 1; i >= 0; i-- {
		var d big.Int
		n.DivMod(n, big.NewInt(62), &d)
		out[i] = base62[d.Int64()]
	}
	return string(out), nil
}

// isHex says whether s is all hex digits in either case.
func isHex(s string) bool {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return s != ""
}
