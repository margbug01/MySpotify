package player

import (
	"encoding/binary"
	"errors"
	"fmt"
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
	IVSize int    // bytes of IV each sample carries, 0 for a constant IV (cbcs)
	KeyID  []byte // the default key's ID, 16 bytes
}

// errNotEncrypted is what a file without a protection of its own is read as.
var errNotEncrypted = errors.New("player: not encrypted")

// encryptionOf reads what the start of a file says its samples are
// encrypted with: its moov, which names the scheme (schm) and the defaults
// of the protection (tenc). head may go on past it, into the samples, as a
// prefetched head does; only the moov is read.
func encryptionOf(head []byte) (Encryption, error) {
	moov := topBox(head, "moov")
	if moov == nil {
		return Encryption{}, errNotEncrypted
	}
	tenc := boxOf(moov, "tenc")
	if tenc == nil {
		return Encryption{}, errNotEncrypted
	}
	// A full box (ISO/IEC 23001-7, 8.2): its version and flags; a reserved
	// byte; another, or the pattern of a version 1 box; whether the samples
	// are protected; the bytes of IV each carries; the key's ID.
	b := tenc[8:]
	if len(b) < 24 {
		return Encryption{}, errors.New("player: tenc is short")
	}
	if b[6] != 1 {
		return Encryption{}, errors.New("player: tenc names no protection")
	}
	iv := int(b[7])
	if iv != 0 && iv != 8 && iv != 16 {
		return Encryption{}, fmt.Errorf("player: tenc names an IV of %d bytes", iv)
	}
	enc := Encryption{Scheme: "cenc", IVSize: iv, KeyID: append([]byte(nil), b[8:24]...)}
	// The scheme the protection names, past schm's version and flags: cenc
	// is the default of a file that names none.
	if schm := boxOf(moov, "schm"); len(schm) >= 16 {
		enc.Scheme = string(schm[12:16])
	}
	return enc, nil
}

// topBox returns the box of type typ among those b is a run of, header and
// all, or nil: b is the start of a file, its boxes one after another (ftyp,
// moov, then the samples' own), and the last may be cut short.
func topBox(b []byte, typ string) []byte {
	for len(b) >= 8 {
		size := uint64(binary.BigEndian.Uint32(b))
		head := uint64(8)
		switch size {
		case 0: // to the end of the file
			size = uint64(len(b))
		case 1: // the size is 64 bits, after the type
			if len(b) < 16 {
				return nil
			}
			size, head = binary.BigEndian.Uint64(b[8:16]), 16
		}
		if size < head {
			return nil
		}
		if string(b[4:8]) == typ {
			if size > uint64(len(b)) {
				return nil
			}
			return b[:size]
		}
		if size >= uint64(len(b)) {
			return nil
		}
		b = b[size:]
	}
	return nil
}

// boxOf returns the first box of type typ anywhere in moov, header and all,
// or nil.
//
// A moov is a kilobyte or two of what the file is, with no samples in it, so
// a search for the type is enough: the boxes on the way to tenc (trak, mdia,
// minf, stbl, stsd, an encrypted sample entry, sinf, schi) need not be
// walked one by one, and a box's type is four bytes nothing else in a moov
// spells by accident.
func boxOf(moov []byte, typ string) []byte {
	for i := 4; i+4 <= len(moov); i++ {
		if string(moov[i:i+4]) != typ {
			continue
		}
		start := i - 4
		size := int(binary.BigEndian.Uint32(moov[start:i]))
		if size < 8 || start+size > len(moov) {
			return nil
		}
		return moov[start : start+size]
	}
	return nil
}
