package spotify

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The web player signs its /api/token requests with a one-time code made
// from a secret it carries in its bundle. Spotify rotates that secret every
// few months: the bundle keeps the current version and the two before it so
// that a cached page keeps working when the live one moves on, and the
// player takes the first, the newest.
//
// Each secret is stored XORed with its own index, as a string of decimal
// numbers: the player maps every character to its code point XORed with
// (i%33)+9, joins those as digits, and reads the result back as hex. The
// key it ends up with is therefore that digit string, not the bytes those
// digits stand for.

// totpSecret is one secret the web player carries: the algorithm obfuscates
// it, and the version both sides name in the request.
type totpSecret struct {
	masked  string
	version int
}

// totpSecrets are what the current web player carries, newest first. The
// first is what it uses; the others are what it falls back to if Spotify
// still honours them after a rotation.
var totpSecrets = []totpSecret{
	{`,7/*F("rLJ2oxaKL^f+E1xvP@N`, 61},
	{`OmE{ZA.J^":0FG\Uz?[@WW`, 60},
	{`{iOFn;4}<1PFYKPV?5{%u14]M>/V0hDH`, 59},
}

// unmask reverses the obfuscation: each character's code point XORed with
// (i%33)+9, joined as decimal digits, and those digits taken as the key.
func (s totpSecret) unmask() []byte {
	var b strings.Builder
	for i, c := range []byte(s.masked) {
		b.WriteString(strconv.Itoa(int(c) ^ ((i % 33) + 9)))
	}
	return []byte(b.String())
}

// hotp is RFC 4226, as the web player's otpauth library computes it: HMAC
// SHA1 over the counter, six digits.
func hotp(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1e6
	return fmt.Sprintf("%06d", code)
}

// totp is the code of the 30-second step ms falls in.
func totp(key []byte, ms int64) string {
	return hotp(key, uint64(ms/1000/30))
}

// tokenQuery is the query /api/token wants: the code of now, and the code of
// Spotify's clock, which must both be right.
func tokenQuery(key []byte, version int, now, serverTime time.Time) string {
	return "reason=init&productType=web_player" +
		"&totp=" + totp(key, now.UnixMilli()) +
		"&totpServer=" + totp(key, serverTime.UnixMilli()) +
		"&totpVer=" + strconv.Itoa(version)
}
