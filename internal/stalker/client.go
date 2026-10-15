// Package stalker talks to Stalker / Ministra ("MAG") middleware portals:
// endpoint discovery, STB authentication, live channel listing, temporary
// stream links and the portal's own programme guide.
//
// The wire behaviour follows IPTVnator's implementation (see its
// docs/architecture/stalker-portal.md): MAG250 identity headers, the
// handshake/get_profile dance, one re-authentication on an auth failure and
// lenient decoding of the portal's loosely typed JSON.
package stalker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	UserAgent = "Mozilla/5.0 (QtEmbedded; U; Linux; C) AppleWebKit/533.3 (KHTML, like Gecko) MAG250"
	timezone  = "Europe/Berlin"
	stbVer    = "ImageDescription: 0.2.18-r14-pub-250; ImageDate: Fri Jan 15 15:20:44 EET 2016; PORTAL version: 5.6.0; API Version: JS API version: 328; STB API version: 134; Player Engine version: 0x566"
)

// Identity is what the portal knows the "set-top box" by. Only the MAC is
// required; the rest is sent when the provider handed it out. Device ids are
// never invented: the portal pins the first value it sees to the MAC.
type Identity struct {
	MAC       string
	Serial    string
	DeviceID  string
	DeviceID2 string
	Login     string
	Password  string
}

// Session is the part of the client state worth keeping between runs.
type Session struct {
	Endpoint string `json:"endpoint,omitempty"`
	Simple   bool   `json:"simple,omitempty"` // token-free panel
	Token    string `json:"token,omitempty"`
}

type Client struct {
	Portal string
	ID     Identity
	HTTP   *http.Client

	mu        sync.Mutex
	sess      Session
	authed    bool
	random    string
	watchdog  time.Duration
	onSession func(Session)
	profile   json.RawMessage // account_info from the last get_profile

	wdMu         sync.Mutex
	lastResolve  time.Time
	watchdogLive bool
}

var (
	ErrAuth = errors.New("портал не принял приставку")
	macRe   = regexp.MustCompile(`^[0-9A-F]{12}$`)
)

// NormalizeMAC accepts 001a79aabbcc, 00-1A-79-…, 00:1a:79:… and returns
// 00:1A:79:AA:BB:CC.
func NormalizeMAC(s string) (string, error) {
	hexOnly := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.TrimSpace(s)))
	if !macRe.MatchString(hexOnly) {
		return "", fmt.Errorf("MAC-адрес %q не похож на 00:1A:79:XX:XX:XX", s)
	}
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = hexOnly[i*2 : i*2+2]
	}
	return strings.Join(parts, ":"), nil
}

func New(portal string, id Identity, sess Session, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{Portal: portal, ID: id, HTTP: httpClient, sess: sess, watchdog: 120 * time.Second}
}

// OnSession is called whenever the endpoint or token changes, so the caller
// can persist them.
func (c *Client) OnSession(f func(Session)) { c.onSession = f }

func (c *Client) Session() Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sess
}

// ---------------------------------------------------------------- requests

func prehash(mac string) string {
	sum := sha1.Sum([]byte(strings.ToUpper(mac)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func cfduid(serial string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(serial) {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' {
			b.WriteRune(r)
		}
	}
	s := b.String() + "e030245495acd6ebfc1"
	if len(s) > 32 {
		return s[:32]
	}
	return s + strings.Repeat("0", 32-len(s))
}

// encodeCmd keeps cmd mostly raw, like a real MAG does; only characters that
// would break the query are escaped.
func encodeCmd(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.~!*()/:?@$,+=[]%", r)) {
			b.WriteRune(r)
			continue
		}
		for _, by := range []byte(string(r)) {
			fmt.Fprintf(&b, "%%%02X", by)
		}
	}
	return b.String()
}

type param struct{ k, v string }

func buildQuery(params []param) string {
	var parts []string
	hasJS := false
	for _, p := range params {
		if p.k == "JsHttpRequest" {
			hasJS = true
		}
		if p.k == "cmd" {
			parts = append(parts, "cmd="+encodeCmd(p.v))
		} else {
			parts = append(parts, url.QueryEscape(p.k)+"="+url.QueryEscape(p.v))
		}
	}
	if !hasJS {
		parts = append(parts, "JsHttpRequest=1-xml")
	}
	return strings.Join(parts, "&")
}

var authFailureText = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^authorization\s+failed[.!]*(?:\s+\d+)?$`),
	regexp.MustCompile(`(?i)^access\s+denied[.!]*$`),
	regexp.MustCompile(`(?i)^unauthorized\s+request[.!]*$`),
}

func isAuthFailure(status int, body []byte) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	text := strings.TrimSpace(string(body))
	if len(text) <= 64 {
		for _, re := range authFailureText {
			if re.MatchString(text) {
				return true
			}
		}
	}
	var env struct {
		JS json.RawMessage `json:"js"`
	}
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	var s string
	if json.Unmarshal(env.JS, &s) == nil {
		return authPhrase(s)
	}
	var obj struct {
		Error any `json:"error"`
		Msg   any `json:"msg"`
	}
	if json.Unmarshal(env.JS, &obj) == nil {
		return authPhrase(fmt.Sprint(obj.Error)) || authPhrase(fmt.Sprint(obj.Msg))
	}
	return false
}

func authPhrase(s string) bool {
	s = strings.ToLower(s)
	for _, p := range []string{"authorization failed", "access denied", "unauthorized", "auth failed", "invalid token"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

type httpError struct {
	status int
	body   []byte
}

func (e *httpError) Error() string {
	return fmt.Sprintf("портал ответил: %d %s", e.status, http.StatusText(e.status))
}

// raw sends one request to endpoint and returns the body.
func (c *Client) raw(ctx context.Context, endpoint string, params []param, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+buildQuery(params), nil)
	if err != nil {
		return 0, nil, err
	}
	cookie := "mac=" + c.ID.MAC + "; stb_lang=en; timezone=" + timezone
	if c.ID.Serial != "" {
		cookie += "; __cfduid=" + cfduid(c.ID.Serial)
		req.Header.Set("SN", c.ID.Serial)
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return resp.StatusCode, body, err
}

// js extracts the {"js": …} payload.
func js(body []byte) (json.RawMessage, error) {
	var env struct {
		JS json.RawMessage `json:"js"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.JS == nil {
		text := strings.TrimSpace(string(body))
		if len(text) > 120 {
			text = text[:120] + "…"
		}
		return nil, fmt.Errorf("портал ответил непонятно: %q", text)
	}
	return env.JS, nil
}

// Call performs an API call, authenticating first if needed and once more if
// the portal says the token is no longer valid.
func (c *Client) Call(ctx context.Context, params ...param) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		if err := c.ensure(ctx); err != nil {
			return nil, err
		}
		c.mu.Lock()
		endpoint, token := c.sess.Endpoint, ""
		if !c.sess.Simple {
			token = c.sess.Token
		}
		c.mu.Unlock()

		status, body, err := c.raw(ctx, endpoint, params, token)
		if err != nil {
			return nil, fmt.Errorf("нет связи с порталом: %w", err)
		}
		if isAuthFailure(status, body) {
			if attempt == 0 {
				c.mu.Lock()
				c.authed = false
				c.sess.Token = ""
				c.mu.Unlock()
				continue
			}
			return nil, fmt.Errorf("%w: %s", ErrAuth, strings.TrimSpace(string(body)))
		}
		if status != http.StatusOK {
			return nil, &httpError{status, body}
		}
		return js(body)
	}
}

func p(k, v string) param { return param{k, v} }

// ---------------------------------------------------------------- discovery & auth

// Candidates lists the API endpoints worth trying for a portal URL as people
// paste it ("…/c/", "…/stalker_portal", or the .php itself).
func Candidates(portal string) ([]string, error) {
	s := strings.TrimSpace(portal)
	if s != "" && !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("адрес портала %q не похож на ссылку", portal)
	}
	u.RawQuery, u.Fragment = "", ""
	path := strings.TrimRight(u.Path, "/")
	origin := u.Scheme + "://" + u.Host

	var out []string
	add := func(p string) {
		for _, o := range out {
			if o == origin+p {
				return
			}
		}
		out = append(out, origin+p)
	}
	isPHP := regexp.MustCompile(`(?i)\.php$`)
	var base string
	if isPHP.MatchString(path) {
		add(path)
		base = regexp.MustCompile(`(?i)/server/load\.php$`).ReplaceAllString(path, "")
		base = regexp.MustCompile(`(?i)/[^/]*\.php$`).ReplaceAllString(base, "")
	} else {
		base = regexp.MustCompile(`(?i)/c$`).ReplaceAllString(path, "")
	}
	add(base + "/portal.php")
	add(base + "/server/load.php")
	if !regexp.MustCompile(`(?i)/stalker_portal(/|$)`).MatchString(base) {
		add(base + "/stalker_portal/server/load.php")
	}
	return out, nil
}

// hasCatalog reports whether a token-less get_genres answer carried data,
// which marks a "simple" panel that needs no authentication.
func hasCatalog(payload json.RawMessage) bool {
	var arr []json.RawMessage
	if json.Unmarshal(payload, &arr) == nil {
		return true
	}
	var obj struct {
		Data  []json.RawMessage `json:"data"`
		Error any               `json:"error"`
	}
	return json.Unmarshal(payload, &obj) == nil && obj.Data != nil && obj.Error == nil
}

func (c *Client) setSession(f func(*Session)) {
	f(&c.sess)
	if c.onSession != nil {
		s := c.sess
		go c.onSession(s)
	}
}

// ensure finds the endpoint and authenticates when that has not happened yet.
func (c *Client) ensure(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authed {
		return nil
	}
	if c.sess.Endpoint == "" {
		if err := c.discover(ctx); err != nil {
			return err
		}
		if c.authed {
			return nil
		}
	}
	if c.sess.Simple {
		c.authed = true
		return nil
	}
	return c.authenticate(ctx)
}

func (c *Client) discover(ctx context.Context) error {
	cands, err := Candidates(c.Portal)
	if err != nil {
		return err
	}
	var last error
	for _, endpoint := range cands {
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		status, body, err := c.raw(pctx, endpoint, []param{p("type", "itv"), p("action", "get_genres")}, "")
		cancel()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				last = err
				continue
			}
			// Refused / DNS: every candidate shares the host.
			return fmt.Errorf("нет связи с порталом: %w", err)
		}
		switch {
		case isAuthFailure(status, body):
			c.setSession(func(s *Session) { s.Endpoint, s.Simple = endpoint, false })
			if err := c.authenticate(ctx); err != nil {
				return err
			}
			return nil
		case status == http.StatusOK:
			if payload, err := js(body); err == nil && hasCatalog(payload) {
				c.setSession(func(s *Session) { s.Endpoint, s.Simple, s.Token = endpoint, true, "" })
				c.authed = true
				return nil
			}
			last = fmt.Errorf("%s: не портал Stalker", endpoint)
		default:
			last = fmt.Errorf("%s: %d %s", endpoint, status, http.StatusText(status))
		}
	}
	if last == nil {
		last = errors.New("портал не найден")
	}
	return fmt.Errorf("не удалось найти API портала: %w", last)
}

type flex string

func (f *flex) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flex(s)
		return nil
	}
	if string(b) == "null" {
		*f = ""
		return nil
	}
	*f = flex(strings.Trim(string(b), `"`))
	return nil
}

func (f flex) int() int {
	if f == "true" {
		return 1
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(f)))
	return n
}

func (f flex) truthy() bool { return f == "true" || f.int() != 0 }

// authenticate runs handshake + get_profile (+ do_auth when asked). Caller
// holds c.mu.
func (c *Client) authenticate(ctx context.Context) error {
	ph := prehash(c.ID.MAC)
	status, body, err := c.raw(ctx, c.sess.Endpoint, []param{
		p("type", "stb"), p("action", "handshake"), p("token", c.sess.Token), p("prehash", ph),
	}, "")
	if err != nil {
		return fmt.Errorf("нет связи с порталом: %w", err)
	}
	if status != http.StatusOK || isAuthFailure(status, body) {
		return fmt.Errorf("%w: рукопожатие не удалось (%d)", ErrAuth, status)
	}
	payload, err := js(body)
	if err != nil {
		return err
	}
	var hs struct {
		Token    string `json:"token"`
		Random   string `json:"random"`
		NotValid flex   `json:"not_valid"`
	}
	if json.Unmarshal(payload, &hs) != nil || hs.Token == "" {
		return fmt.Errorf("%w: портал не выдал токен", ErrAuth)
	}
	c.random = hs.Random
	if c.random == "" {
		b := make([]byte, 20)
		for i := range b {
			b[i] = byte(rand.IntN(256))
		}
		c.random = hex.EncodeToString(b)
	}
	token := hs.Token
	notValid := "0"
	if hs.NotValid.truthy() {
		notValid = "1"
	}

	profile := func(secondStep string) (map[string]json.RawMessage, error) {
		metrics := map[string]string{"mac": c.ID.MAC, "model": "MAG250", "type": "STB", "random": c.random}
		if c.ID.Serial != "" {
			metrics["sn"] = c.ID.Serial
		}
		m, _ := json.Marshal(metrics)
		params := []param{
			p("type", "stb"), p("action", "get_profile"),
			p("hd", "1"), p("ver", stbVer), p("num_banks", "2"), p("stb_type", "MAG250"),
			p("client_type", "STB"), p("image_version", "218"), p("video_out", "hdmi"),
			p("hw_version", "1.7-BD-00"), p("not_valid_token", notValid), p("auth_second_step", secondStep),
			p("metrics", string(m)), p("prehash", ph),
		}
		for _, kv := range []param{{"sn", c.ID.Serial}, {"device_id", c.ID.DeviceID}, {"device_id2", c.ID.DeviceID2}} {
			if kv.v != "" {
				params = append(params, kv)
			}
		}
		status, body, err := c.raw(ctx, c.sess.Endpoint, params, token)
		if err != nil {
			return nil, fmt.Errorf("нет связи с порталом: %w", err)
		}
		if status != http.StatusOK || isAuthFailure(status, body) {
			return nil, fmt.Errorf("%w (%s)", ErrAuth, strings.TrimSpace(string(body)))
		}
		payload, err := js(body)
		if err != nil {
			return nil, err
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(payload, &obj) != nil {
			return nil, fmt.Errorf("%w: непонятный ответ на get_profile", ErrAuth)
		}
		return obj, nil
	}

	obj, err := profile("0")
	if err != nil {
		return err
	}
	str := func(k string) string {
		var f flex
		json.Unmarshal(obj[k], &f)
		return strings.TrimSpace(string(f))
	}
	switch {
	case str("status") == "2":
		if c.ID.Login == "" {
			return fmt.Errorf("%w: портал просит логин и пароль — укажите их в настройках плейлиста", ErrAuth)
		}
		_, body, err := c.raw(ctx, c.sess.Endpoint, []param{
			p("type", "stb"), p("action", "do_auth"), p("login", c.ID.Login), p("password", c.ID.Password),
			p("device_id", c.ID.DeviceID), p("device_id2", c.ID.DeviceID2),
		}, token)
		if err != nil {
			return fmt.Errorf("нет связи с порталом: %w", err)
		}
		if payload, err := js(body); err != nil || strings.TrimSpace(string(payload)) != "true" {
			return fmt.Errorf("%w: неверный логин или пароль", ErrAuth)
		}
		if obj, err = profile("1"); err != nil {
			return err
		}
		if s := str("status"); s != "" && s != "0" {
			return fmt.Errorf("%w: вход не подтверждён (статус %s)", ErrAuth, s)
		}
	case str("status") != "" && str("status") != "0", str("msg") != "", str("block_msg") != "":
		reason := str("block_msg")
		if reason == "" {
			reason = str("msg")
		}
		if reason == "" {
			reason = "MAC-адрес не зарегистрирован или заблокирован"
		}
		return fmt.Errorf("%w: %s", ErrAuth, reason)
	}
	c.profile = obj["account_info"]
	if wd := flex(str("watchdog_timeout")).int(); wd > 0 {
		c.watchdog = time.Duration(min(max(wd, 30), 3600)) * time.Second
	}
	c.setSession(func(s *Session) { s.Token = token })
	c.authed = true
	return nil
}
