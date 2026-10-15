package enigma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebIf is a tiny OpenWebif client. On the receiver itself it talks to
// http://127.0.0.1.
type WebIf struct {
	BaseURL  string
	User     string
	Password string
}

func (w WebIf) Enabled() bool { return strings.TrimSpace(w.BaseURL) != "" }

func (w WebIf) get(ctx context.Context, path string, q url.Values, timeout time.Duration, out any) error {
	if !w.Enabled() {
		return errors.New("адрес OpenWebif не задан в настройках")
	}
	u := strings.TrimRight(w.BaseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if w.User != "" {
		req.SetBasicAuth(w.User, w.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("нет связи с ресивером: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errors.New("OpenWebif требует логин и пароль — укажите их в настройках")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenWebif ответил: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type result struct {
	Result  bool   `json:"result"`
	Message string `json:"message"`
}

func (r result) err(what string) error {
	if r.Result {
		return nil
	}
	if r.Message == "" {
		r.Message = "неизвестная ошибка"
	}
	return fmt.Errorf("%s: %s", what, r.Message)
}

// ReloadBouquets makes Enigma2 re-read userbouquets without restarting.
func (w WebIf) ReloadBouquets(ctx context.Context) error {
	var r result
	if err := w.get(ctx, "/api/servicelistreload", url.Values{"mode": {"2"}}, 30*time.Second, &r); err != nil {
		return err
	}
	return r.err("ресивер не перечитал списки")
}

type BoxStatus struct {
	Standby bool   `json:"standby"`
	Current string `json:"current"`
}

func (w WebIf) Status(ctx context.Context) (BoxStatus, error) {
	var raw struct {
		InStandby   any    `json:"inStandby"`
		CurrentName string `json:"currservice_name"`
	}
	if err := w.get(ctx, "/api/statusinfo", nil, 5*time.Second, &raw); err != nil {
		return BoxStatus{}, err
	}
	return BoxStatus{Standby: fmt.Sprint(raw.InStandby) == "true", Current: raw.CurrentName}, nil
}

// Zap switches the TV to ref, waking the receiver from standby first.
func (w WebIf) Zap(ctx context.Context, ref string) error {
	if st, err := w.Status(ctx); err == nil && st.Standby {
		var r result
		_ = w.get(ctx, "/api/powerstate", url.Values{"newstate": {"4"}}, 10*time.Second, &r)
	}
	var r result
	if err := w.get(ctx, "/api/zap", url.Values{"sRef": {ref}}, 15*time.Second, &r); err != nil {
		return err
	}
	return r.err("не удалось включить канал")
}
