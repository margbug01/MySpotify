package qtweb

import (
	"os"
	"path/filepath"
)

func ProfileDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "MySpotify", "qt-webengine"), nil
}

// Only non-secret migration/login markers live here. Cookies stay in the browser's store.
func state(name string) bool {
	dir, err := ProfileDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, name))
	return err == nil
}

func setState(name string, enabled bool) error {
	dir, err := ProfileDir()
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte("1\n"), 0600)
}

func Migrated() bool                    { return state("native-migrated") }
func MarkMigrated() error               { return setState("native-migrated", true) }
func KnownLogin() bool                  { return state("has-login") }
func SetKnownLogin(signedIn bool) error { return setState("has-login", signedIn) }
