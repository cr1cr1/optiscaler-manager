package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newPCGWStub(t *testing.T, pageTitle string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if pageTitle == "" {
			_, _ = fmt.Fprint(w, `["q",[],[],[]]`)
			return
		}
		fmt.Fprintf(w, `["q",[%q],["d"],["u"]]`, pageTitle)
	}))
	t.Cleanup(srv.Close)
	return srv
}
