package server

import (
	"dreaminator/internal/app"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogRoutesErrors(t *testing.T) {
	a, err := app.New(app.Config{DataDir: t.TempDir()}, app.DefaultSettings(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	h := New(a, fstest.MapFS{})
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/catalog/sources", "", 200},
		{"GET", "/api/catalog/missing/items?type=vod&page=1", "", 404},
		{"GET", "/api/catalog/missing/items?type=vod&page=garbage", "", 400},
		{"POST", "/api/catalog/missing/play", "{bad", 400},
		{"GET", "/api/catalog/missing/epg?key=k", "", 404},
		{"GET", "/api/catalog/missing/epg?key=k&day=x", "", 400},
		{"GET", "/api/catalog/missing/epg?key=k&day=0&tz=9999", "", 400},
		{"GET", "/api/catalog/missing/epg?key=k&id=missing/itv/1", "", 404},
		{"GET", "/api/catalog/service?ref=1%3A0%3A19%3A283D%3A3FB%3A1%3AC00000%3A0%3A0%3A0%3A", "", 404},
		{"GET", "/api/catalog/service?ref=4097%3A0%3A1%3A0%3A0%3A0%3A0%3A0%3A0%3A0%3Ahttp%253a//h/x.ts%3AX", "", 404},
		{"GET", "/api/catalog/missing/now?keys=a,b", "", 404},
		{"GET", "/api/catalog/missing/now?keys=" + strings.Repeat("k,", 70), "", 400},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
