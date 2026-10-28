package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
)

// The cookies are sealed with AES-GCM, under a key the login keychain keeps:
// the file alone, in a backup or copied off the machine, says nothing.
// WebKit keeps its own cookies in the clear; these are no easier to read.

// keychainService names the key's item in the login keychain; tests name
// one of their own.
var keychainService = "dev.margbug.vinyl"

const keychainAccount = "account-cookies"

var (
	keyMu     sync.Mutex
	cookieKey []byte // the key, once the keychain gave it
)

// accountCookieDir is where the cookies are kept: beside the settings.
func accountCookieDir() (string, error) { return mygo.App.Path(mygo.PathUserData) }

// sealKey returns the key, made and kept in the login keychain the first
// time. The security tool keeps it, so that it, and not this build, is what
// the item lets read it: an update of the app is not asked about it again.
func sealKey() ([]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	if cookieKey != nil {
		return cookieKey, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	chain := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	find := []string{"find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w", chain}
	out, err := exec.Command("/usr/bin/security", find...).Output()
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(out)))
		if err != nil || len(key) != 32 {
			return nil, errors.New("keychain: the key is not one")
		}
		cookieKey = key
		return key, nil
	}
	// 44: no such item, which is the first time.
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 44 {
		return nil, fmt.Errorf("keychain: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// Through the tool's own prompt, so that the key is in no command line.
	add := exec.Command("/usr/bin/security", "-i")
	add.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -s %s -a %s -w %s %s\n",
		strconv.Quote(keychainService), keychainAccount, hex.EncodeToString(key), strconv.Quote(chain)))
	if _, err := add.Output(); err != nil {
		return nil, fmt.Errorf("keychain: keeping the key: %w", err)
	}
	// The prompt may say nothing of a command that failed: read it back.
	out, err = exec.Command("/usr/bin/security", find...).Output()
	if err != nil || strings.TrimSpace(string(out)) != hex.EncodeToString(key) {
		return nil, errors.New("keychain: the key kept does not read back")
	}
	cookieKey = key
	return key, nil
}

func cookieCipher() (cipher.AEAD, error) {
	key, err := sealKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal encrypts b: a nonce, then what it sealed.
func seal(b []byte) ([]byte, error) {
	gcm, err := cookieCipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, b, nil), nil
}

// unseal opens what seal sealed, failing with errUnreadable for anything
// else, such as the cookies a build before this one kept in the clear.
func unseal(b []byte) ([]byte, error) {
	gcm, err := cookieCipher()
	if err != nil {
		return nil, err
	}
	if len(b) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errUnreadable
	}
	out, err := gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], nil)
	if err != nil {
		return nil, errUnreadable
	}
	return out, nil
}
