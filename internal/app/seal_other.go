//go:build !darwin && !windows

package app

import (
	"errors"

	"github.com/egoist/mygo"
)

// Here there is no key of the login to seal the cookies with, and they are
// not kept: every renewal goes through the web view, as before they were.

func accountCookieDir() (string, error) { return mygo.App.Path(mygo.PathUserData) }

func seal([]byte) ([]byte, error) { return nil, errors.New("no key store to seal them with") }

func unseal([]byte) ([]byte, error) { return nil, errUnreadable }
