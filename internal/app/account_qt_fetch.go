package app

import (
	"context"
	"encoding/json"
	"time"

	"vinyl/internal/qtweb"
)

// The browser sends its own HttpOnly cookies. Only the token response crosses
// private stdio; a full web-player page and its background workers are unnecessary.
func qtTokenBody(parent context.Context, client *qtweb.Client, id uint64, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, "auth.open", map[string]any{"id": id, "url": url, "preload": ""}); err != nil {
		return nil, err
	}
	for {
		raw, err := client.Call(ctx, "auth.eval", map[string]any{"id": id, "script": `location.pathname === '/api/token' ? (document.querySelector('pre')?.textContent || '') : ''`})
		if err != nil {
			return nil, err
		}
		var text string
		if json.Unmarshal(raw, &text) == nil && json.Valid([]byte(text)) {
			return []byte(text), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
