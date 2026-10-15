package stalker

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Account is what the portal says about the subscription. Portals disagree
// on field names and date formats, so everything is optional.
type Account struct {
	Login     string
	Plan      string
	Active    *bool
	Expires   *time.Time
	Unlimited bool
	// RawExpiry keeps a date the parser did not understand, to show as is.
	RawExpiry string
}

var dateLayouts = []string{
	"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02", "02.01.2006 15:04", "02.01.2006",
	"January 2, 2006, 3:04 pm", "January 2, 2006, 15:04", "January 2, 2006", "Jan 2, 2006",
}

// parseExpiry understands unix seconds and milliseconds, -1/0 for "no end"
// and the date strings portals print.
func parseExpiry(v string) (t *time.Time, unlimited, ok bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, false, false
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		switch {
		case n <= 0:
			return nil, true, true
		case n > 1e12:
			t := time.UnixMilli(n)
			return &t, false, true
		case n > 1e9:
			t := time.Unix(n, 0)
			return &t, false, true
		}
		return nil, false, false
	}
	loc, _ := time.LoadLocation(timezone)
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			if t.Year() < 2000 {
				return nil, true, true
			}
			return &t, false, true
		}
	}
	return nil, false, false
}

func decodeAccount(raw json.RawMessage) (Account, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return Account{}, false
	}
	if nested, ok := obj["account_info"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(nested, &inner) == nil {
			for k, v := range inner {
				obj[k] = v
			}
		}
	}
	var acc Account
	found := false
	acc.Login = field(obj, "login", "fname")
	acc.Plan = field(obj, "tariff_plan_name", "tariff_plan")
	if s := field(obj, "status"); s != "" {
		active := flex(s).int() == 1 || strings.EqualFold(s, "active")
		acc.Active = &active
		found = true
	}
	for _, k := range []string{"expire_date", "end_date", "expire_billing_date", "phone"} {
		v := field(obj, k)
		if v == "" {
			continue
		}
		if t, unlimited, ok := parseExpiry(v); ok {
			acc.Expires, acc.Unlimited = t, unlimited
			found = true
			break
		}
		if k != "phone" && acc.RawExpiry == "" {
			acc.RawExpiry = v
			found = true
		}
	}
	return acc, found || acc.Login != "" || acc.Plan != ""
}

// Account asks get_main_info and falls back to the account_info that
// get_profile returned during login.
func (c *Client) Account(ctx context.Context) (Account, error) {
	payload, err := c.Call(ctx, p("type", "account_info"), p("action", "get_main_info"))
	if err == nil {
		if acc, ok := decodeAccount(payload); ok {
			return acc, nil
		}
	}
	c.mu.Lock()
	profile := c.profile
	c.mu.Unlock()
	if acc, ok := decodeAccount(profile); ok {
		return acc, nil
	}
	if err != nil {
		return Account{}, err
	}
	return Account{}, nil
}
