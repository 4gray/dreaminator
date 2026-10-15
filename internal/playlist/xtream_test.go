package playlist

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"dreaminator/internal/fetch"
)

func TestXtreamAccount(t *testing.T) {
	exp := time.Date(2027, 3, 12, 0, 0, 0, 0, time.UTC).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("username") {
		case "dad":
			fmt.Fprintf(w, `{"user_info":{"auth":1,"status":"Active","exp_date":"%d","is_trial":"0","active_cons":"1","max_connections":"2"}}`, exp)
		case "forever":
			fmt.Fprint(w, `{"user_info":{"auth":1,"status":"Active","exp_date":null,"active_cons":0,"max_connections":1}}`)
		case "old":
			fmt.Fprint(w, `{"user_info":{"auth":1,"status":"Expired","exp_date":"1600000000"}}`)
		default:
			fmt.Fprint(w, `{"user_info":{"auth":0}}`)
		}
	}))
	defer srv.Close()
	ctx, o := context.Background(), fetch.Options{Timeout: 5 * time.Second}

	acc, err := Xtream{Server: srv.URL, Username: "dad", Password: "x"}.Account(ctx, o)
	if err != nil || !acc.Active || acc.Expires.Unix() != exp || acc.ActiveCons != 1 || acc.MaxConnections != 2 {
		t.Fatalf("dad = %+v, %v", acc, err)
	}
	if acc, _ := (Xtream{Server: srv.URL, Username: "forever", Password: "x"}).Account(ctx, o); !acc.Unlimited || acc.Expires != nil {
		t.Fatalf("forever = %+v", acc)
	}
	acc, err = Xtream{Server: srv.URL, Username: "old", Password: "x"}.Account(ctx, o)
	if err != nil || acc.Active || acc.Status != "Expired" {
		t.Fatalf("old = %+v, %v", acc, err)
	}
	if _, err := (Xtream{Server: srv.URL, Username: "nope", Password: "x"}).Account(ctx, o); err == nil {
		t.Fatal("bad login accepted")
	}
	if _, err := (Xtream{Server: srv.URL, Username: "old", Password: "x"}).Fetch(ctx, o); err == nil {
		t.Fatal("Fetch must refuse an expired subscription")
	}
}

func TestXtreamShortEPG(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	var asked url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		switch r.URL.Query().Get("stream_id") {
		case "7":
			fmt.Fprintf(w, `{"epg_listings":[
				{"id":"2","title":"%s","description":"%s","start":"2026-10-05 19:00:00","end":"2026-10-05 20:30:00","start_timestamp":"1759690800"},
				{"id":"1","title":"%s","description":"","start":"2026-10-05 18:00:00","end":"2026-10-05 19:00:00","start_timestamp":"1759687200","stop_timestamp":"1759690800"},
				{"id":"3","title":"","start_timestamp":"1759696200","stop_timestamp":"1759699800"},
				{"id":"4","title":"Plain text","start_timestamp":1759696200,"stop_timestamp":1759699800}]}`,
				b64("Фильм «Ёлки»"), b64("Описание\nв две строки"), b64("Новости"))
		case "8":
			fmt.Fprint(w, `[]`)
		default:
			fmt.Fprint(w, `<html>`)
		}
	}))
	defer srv.Close()
	x := Xtream{Server: srv.URL, Username: "u", Password: "p"}
	ctx, o := context.Background(), fetch.Options{Timeout: 5 * time.Second}

	got, err := x.ShortEPG(ctx, o, "7", 10)
	if err != nil {
		t.Fatal(err)
	}
	if asked.Get("action") != "get_short_epg" || asked.Get("limit") != "10" || asked.Get("username") != "u" {
		t.Fatalf("query = %v", asked)
	}
	if len(got) != 3 || got[0].Title != "Новости" || got[1].Title != "Фильм «Ёлки»" || got[2].Title != "Plain text" {
		t.Fatalf("listings = %+v", got)
	}
	if got[0].Stop.Unix() != 1759690800 || got[1].Desc != "Описание\nв две строки" {
		t.Errorf("first = %+v, second = %+v", got[0], got[1])
	}
	// No stop timestamp: the length comes from the panel's own times.
	if d := got[1].Stop.Sub(got[1].Start); d != 90*time.Minute {
		t.Errorf("length without stop_timestamp = %v", d)
	}
	if got, err := x.ShortEPG(ctx, o, "8", 10); err != nil || len(got) != 0 {
		t.Errorf("empty guide = %+v, %v", got, err)
	}
	if _, err := x.ShortEPG(ctx, o, "9", 10); err == nil {
		t.Error("garbage accepted")
	}
}
