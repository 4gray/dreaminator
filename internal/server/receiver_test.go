package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"dreaminator/internal/app"
	"dreaminator/internal/enigma"
)

func TestReceiverAPIEditUndoAndReloadFailure(t *testing.T) {
	reloads := 0
	reloadOK := true
	webif := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/getservices" {
			io.WriteString(w, `{"services":[{"servicereference":"1:0:1:42:0:0:0:0:0:0:","servicename":"DVB name"}]}`)
			return
		}
		if r.URL.Path != "/api/servicelistreload" || r.URL.Query().Get("mode") != "2" {
			t.Errorf("unexpected Webif request %s", r.URL)
		}
		reloads++
		json.NewEncoder(w).Encode(map[string]bool{"result": reloadOK})
	}))
	defer webif.Close()
	dir := t.TempDir()
	e2 := filepath.Join(dir, "e2")
	os.MkdirAll(e2, 0755)
	const id = "userbouquet.native.tv"
	original := "#NAME Original\n#SERVICE 1:0:1:42:0:0:0:0:0:0:\n"
	os.WriteFile(filepath.Join(e2, id), []byte(original), 0644)
	os.WriteFile(filepath.Join(e2, "bouquets.tv"), []byte("#NAME TV\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \""+id+"\" ORDER BY bouquet\n"), 0644)
	defaults := app.DefaultSettings()
	defaults.WebifURL = webif.URL
	a, err := app.New(app.Config{DataDir: filepath.Join(dir, "data"), E2Dir: e2, EPGImportDir: filepath.Join(dir, "epg")}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	handler := New(a, fstest.MapFS{})
	req := func(method, path string, body any, out any) int {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(method, path, bytes.NewReader(data)))
		if out != nil {
			if err := json.Unmarshal(rr.Body.Bytes(), out); err != nil {
				t.Fatalf("decode %s: %v", rr.Body, err)
			}
		}
		return rr.Code
	}
	var list []enigma.ReceiverBouquet
	if code := req("GET", "/api/receiver-bouquets", nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("list %d %+v", code, list)
	}
	var b enigma.ReceiverBouquet
	if code := req("GET", "/api/receiver-bouquets/"+id, nil, &b); code != 200 {
		t.Fatal(code)
	}
	if b.Channels[0].Name != "DVB name" {
		t.Fatalf("DVB label: %+v", b)
	}
	if b.Channels[0].Reference != "" {
		t.Fatal("API exposed raw service URL")
	}
	patch := enigma.ReceiverPatch{Revision: b.Revision, Name: "Edited", Order: []string{b.Channels[0].ID}}
	var result struct {
		Bouquet enigma.ReceiverBouquet `json:"bouquet"`
		Warning string                 `json:"warning"`
	}
	if code := req("PUT", "/api/receiver-bouquets/"+id, patch, &result); code != 200 || reloads != 1 || !result.Bouquet.CanUndo {
		t.Fatalf("save %d %+v reloads %d", code, result, reloads)
	}
	if code := req("PUT", "/api/receiver-bouquets/"+id, patch, nil); code != 409 || reloads != 1 {
		t.Fatalf("stale save %d", code)
	}
	reloadOK = false
	if code := req("POST", "/api/receiver-bouquets/"+id+"/undo", map[string]string{"revision": result.Bouquet.Revision}, &result); code != 200 || result.Warning == "" {
		t.Fatalf("undo reload failure %d %+v", code, result)
	}
	data, _ := os.ReadFile(filepath.Join(e2, id))
	if string(data) != original {
		t.Fatal("undo did not restore original")
	}
	if code := req("POST", "/api/receiver-bouquets/"+id+"/copy", map[string]string{"revision": result.Bouquet.Revision, "name": "Copy"}, &result); code != 200 || result.Bouquet.ID == id || result.Warning == "" {
		t.Fatalf("copy %d %+v", code, result)
	}
	reloadOK = true
	if code := req("POST", "/api/receiver-bouquets/reload", nil, nil); code != 200 {
		t.Fatal(code)
	}
	if code := req("PUT", "/api/receiver-bouquets/"+id, map[string]any{"revision": b.Revision, "name": "Bad", "order": []string{"unknown"}}, nil); code == 200 {
		t.Fatal("invalid accepted")
	}
	data, _ = os.ReadFile(filepath.Join(e2, id))
	if !strings.Contains(string(data), "Original") {
		t.Fatal("invalid write changed original")
	}
}

func TestReceiverAPIRenameAndDelete(t *testing.T) {
	reloads := 0
	reloadOK := true
	webif := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servicelistreload" {
			t.Errorf("unexpected Webif request %s", r.URL)
		}
		reloads++
		json.NewEncoder(w).Encode(map[string]bool{"result": reloadOK})
	}))
	defer webif.Close()
	dir := t.TempDir()
	e2 := filepath.Join(dir, "e2")
	os.MkdirAll(e2, 0755)
	const id, gone = "userbouquet.native.tv", "userbouquet.gone.tv"
	os.WriteFile(filepath.Join(e2, id), []byte("#NAME Original\n#SERVICE 1:0:1:42:0:0:0:0:0:0:\n#DESCRIPTION Named\n"), 0644)
	os.WriteFile(filepath.Join(e2, "bouquets.tv"), []byte("#NAME TV\n"+
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \""+id+"\" ORDER BY bouquet\n"+
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \""+gone+"\" ORDER BY bouquet\n"), 0644)
	defaults := app.DefaultSettings()
	defaults.WebifURL = webif.URL
	a, err := app.New(app.Config{DataDir: filepath.Join(dir, "data"), E2Dir: e2, EPGImportDir: filepath.Join(dir, "epg")}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	handler := New(a, fstest.MapFS{})
	req := func(method, path string, body any, out any) int {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(method, path, bytes.NewReader(data)))
		if out != nil {
			if err := json.Unmarshal(rr.Body.Bytes(), out); err != nil {
				t.Fatalf("decode %s: %v", rr.Body, err)
			}
		}
		return rr.Code
	}
	var list []enigma.ReceiverBouquet
	req("GET", "/api/receiver-bouquets", nil, &list)
	if len(list) != 2 || list[0].Revision == "" {
		t.Fatalf("list %+v", list)
	}
	var result struct {
		Bouquet enigma.ReceiverBouquet `json:"bouquet"`
		Warning string                 `json:"warning"`
	}
	if code := req("POST", "/api/receiver-bouquets/"+id+"/rename", map[string]string{"revision": list[0].Revision, "name": "Renamed"}, &result); code != 200 || result.Bouquet.Name != "Renamed" || reloads != 1 {
		t.Fatalf("rename %d %+v reloads %d", code, result, reloads)
	}
	if code := req("POST", "/api/receiver-bouquets/"+id+"/rename", map[string]string{"revision": list[0].Revision, "name": "Stale"}, nil); code != 409 {
		t.Fatalf("stale rename %d", code)
	}
	if code := req("DELETE", "/api/receiver-bouquets/"+id+"?revision="+list[0].Revision, nil, nil); code != 409 {
		t.Fatalf("stale delete %d", code)
	}
	reloadOK = false
	var deleted struct {
		Warning string `json:"warning"`
	}
	if code := req("DELETE", "/api/receiver-bouquets/"+id+"?revision="+result.Bouquet.Revision, nil, &deleted); code != 200 || deleted.Warning == "" {
		t.Fatalf("delete with failed reload %d %+v", code, deleted)
	}
	if _, err := os.Stat(filepath.Join(e2, id)); !os.IsNotExist(err) {
		t.Fatal("bouquet file still on the receiver")
	}
	reloadOK = true
	if code := req("DELETE", "/api/receiver-bouquets/"+gone, nil, &deleted); code != 200 || deleted.Warning != "" {
		t.Fatalf("delete missing file %d %+v", code, deleted)
	}
	req("GET", "/api/receiver-bouquets", nil, &list)
	if len(list) != 0 {
		t.Fatalf("deleted bouquets still listed: %+v", list)
	}
	index, _ := os.ReadFile(filepath.Join(e2, "bouquets.tv"))
	if string(index) != "#NAME TV\n" {
		t.Fatalf("index %q", index)
	}
}
