package playlist

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
