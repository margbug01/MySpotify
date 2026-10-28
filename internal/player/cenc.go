package player

import (
	"encoding/binary"
	"errors"
)

// Encryption is what a file's samples are encrypted with, as the file's own
// init section says: the scheme its protection is in (cenc, or cbcs for
// SAMPLE-AES), how long the IV each sample carries is, and the key they all
// belong to.
//
// The app does not decrypt with it — the key system does, and its key stays
// there. It is what tells a file the key system in this build cannot play
// (a cenc track where FairPlay is all WebKit has) from one that plays badly
// for another reason, at the start rather than as a still picture.
type Encryption struct {
	Scheme string // the fourcc the protection names: cenc, cbcs, piff
	IVSize int    // bytes of IV each sample carries, 0 for the default IV
	KeyID  []byte // the default key's ID, 16 bytes
}

// errNotEncrypted is what a file without a protection of its own is read as.
var errNotEncrypted = errors.New("player: not encrypted")

// encryptionOf reads what init, the start of a file, says its samples are
// encrypted with. It is the first bytes of the file: the moov, which names
// the scheme (schm) and the defaults of the protection (tenc).
func encryptionOf(init []byte) (Encryption, error) {
	tenc := boxOf(init, "tenc")
	if tenc == nil {
		return Encryption{}, errNotEncrypted
	}
	body := tenc[8:] // past the version and flags
	// Where the protection's defaults start is not the same everywhere: some
	// files carry one byte before them, some two, and both are read here by
	// what the bytes say rather than by counting — the first byte that says
	// the samples are protected, with an IV length that makes sense after it.
	for _, at := range []int{6, 4, 5, 7} {
		if len(body) < at+2+16 || body[at] != 1 {
			continue
		}
		if iv := int(body[at+1]); iv == 0 || iv == 8 || iv == 16 {
			enc := Encryption{Scheme: "cenc", IVSize: iv}
			enc.KeyID = append([]byte(nil), body[at+2:at+2+16]...)
			// The scheme the protection names, when it names one: cenc is the
			// default of a file that does not.
			if schm := boxOf(init, "schm"); len(schm) >= 16 {
				enc.Scheme = string(schm[12:16])
			}
			return enc, nil
		}
	}
	return Encryption{}, errors.New("player: tenc names no protection")
}

// boxOf returns b's first box of type typ, header and all, or nil.
//
// The init section is a kilobyte or two of moov, so a search for the type is
// enough: it cannot find one in sample data, and a box's type is four bytes
// no codec writes by accident.
func boxOf(b []byte, typ string) []byte {
	for i := 4; i+4 <= len(b); i++ {
		if string(b[i:i+4]) != typ {
			continue
		}
		start := i - 4
		size := int(binary.BigEndian.Uint32(b[start:i]))
		if size == 1 && start+16 <= len(b) {
			size = int(binary.BigEndian.Uint64(b[start+8 : start+16]))
		}
		if size < 8 || start+size > len(b) {
			return nil
		}
		return b[start : start+size]
	}
	return nil
}
