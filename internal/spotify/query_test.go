package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A null root with GraphQL errors must fail; usable partial data must survive (#42).
func TestQueryReportsFailedRoots(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		failed, stale  bool
	}{
		{"null data", `{"data":null,"errors":[{"message":"unavailable"}]}`, true, false},
		{"null browse", `{"data":{"browse":null},"errors":[{"message":"unavailable","path":["browse"]}]}`, true, false},
		{"null roots", `{"data":{"browse":null,"home":null},"errors":[{"message":"unavailable"}]}`, true, false},
		{"stale null root", `{"data":{"browse":null},"errors":[{"message":"PersistedQueryNotFound"}]}`, true, true},
		{"usable partial data", `{"data":{"browse":{"sections":{"items":[]}},"other":null},"errors":[{"message":"other unavailable","path":["other"]}]}`, false, false},
		{"empty success", `{"data":{"browse":{"sections":{"items":[]}}}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.response)
			}))
			defer srv.Close()
			pf := pathfinderURL
			pathfinderURL = srv.URL
			SetIdentity(Identity{Token: "t", Expires: time.Now().Add(time.Hour)})
			defer func() { pathfinderURL = pf; SetIdentity(Identity{}) }()
			var out map[string]json.RawMessage
			err := queryAs("probe", "hash", nil, &out)
			if (err != nil) != tc.failed || errors.Is(err, errStaleQuery) != tc.stale {
				t.Fatalf("error = %v, want failed=%v stale=%v", err, tc.failed, tc.stale)
			}
			if !tc.failed && (len(out["browse"]) == 0 || string(out["browse"]) == "null") {
				t.Fatalf("lost usable browse: %s", out["browse"])
			}
		})
	}
}
