package app

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

// The cookies are sealed with DPAPI, as WebView2 seals its own: under a key
// of the listener's Windows login, which another user, or the file copied
// off the machine, cannot open.

var (
	crypt32           = syscall.NewLazyDLL("crypt32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procProtectData   = crypt32.NewProc("CryptProtectData")
	procUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree     = kernel32.NewProc("LocalFree")
	cookieEntropy     = []byte("dev.margbug.vinyl account-cookies")
	errNoDPAPI        = fmt.Errorf("DPAPI: crypt32.dll has none")
)

// cryptprotectUIForbidden: fail rather than ask the listener anything.
const cryptprotectUIForbidden = 0x1

// accountCookieDir is where the cookies are kept: %LOCALAPPDATA%\Vinyl,
// beside WebView2's own, which a roaming profile does not take along to
// other machines as it does %APPDATA%.
func accountCookieDir() (string, error) { return mygo.App.Path(mygo.PathCache) }

type dataBlob struct {
	size uint32
	data *byte
}

func blobOf(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{size: uint32(len(b)), data: &b[0]}
}

// take copies out what Windows wrote to blob, and frees it.
func (b *dataBlob) take() []byte {
	if b.data == nil {
		return nil
	}
	out := append([]byte(nil), unsafe.Slice(b.data, b.size)...)
	procLocalFree.Call(uintptr(unsafe.Pointer(b.data)))
	return out
}

func seal(b []byte) ([]byte, error) {
	if procProtectData.Find() != nil {
		return nil, errNoDPAPI
	}
	in, entropy := blobOf(b), blobOf(cookieEntropy)
	var out dataBlob
	r, _, err := procProtectData.Call(uintptr(unsafe.Pointer(in)), 0, uintptr(unsafe.Pointer(entropy)),
		0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(b)
	if r == 0 {
		return nil, fmt.Errorf("DPAPI: %w", err)
	}
	return out.take(), nil
}

// unseal opens what seal sealed, failing with errUnreadable for anything it
// cannot: another user's, another machine's, or not sealed at all.
func unseal(b []byte) ([]byte, error) {
	if procUnprotectData.Find() != nil {
		return nil, errNoDPAPI
	}
	in, entropy := blobOf(b), blobOf(cookieEntropy)
	var out dataBlob
	r, _, _ := procUnprotectData.Call(uintptr(unsafe.Pointer(in)), 0, uintptr(unsafe.Pointer(entropy)),
		0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(b)
	if r == 0 {
		return nil, errUnreadable
	}
	return out.take(), nil
}
