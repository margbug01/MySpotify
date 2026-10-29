//go:build !darwin

package player

import "testing"

func runTests(m *testing.M) int { return m.Run() }
