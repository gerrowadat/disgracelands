// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// webTestLimits matches serveOn's own telnet test limits: no player cap,
// a generous per-host one, and a login grace long enough that a slow CI
// box never trips it mid-test.
var webTestLimits = Limits{MaxPerHost: 8, LoginGrace: time.Minute}

// listeningWeb builds the web interface's http.Handler on a real
// httptest.Server, torn down with the test.
func listeningWeb(t *testing.T, srv *Server, password string, captcha bool) *httptest.Server {
	t.Helper()
	h, err := srv.WebHandler(context.Background(), WebOptions{
		Password: password, Captcha: captcha, Limits: webTestLimits,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// TestWebIndexServesAWelcomePage: the plain front door, with a way into
// the game.
func TestWebIndexServesAWelcomePage(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Disgracelands") || !strings.Contains(string(body), `href="/play"`) {
		t.Errorf("index page = %q, want it to mention Disgracelands and link to /play", body)
	}
}

// TestWebPasswordGatesEveryRoute: WebPassword is the whole interface's own
// front door, not just /play — set, and nothing behind it is reachable
// without it, including the welcome page.
func TestWebPasswordGatesEveryRoute(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "letmein", false)

	for _, tc := range []struct {
		name string
		user string
		pass string
		want int
	}{
		{"no credentials", "", "", http.StatusUnauthorized},
		{"wrong password", "anyone", "wrong", http.StatusUnauthorized},
		// The username is not checked at all — see requirePassword's own
		// doc comment — so any name with the right password gets in.
		{"right password, any username", "anyone", "letmein", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.user != "" || tc.pass != "" {
				req.SetBasicAuth(tc.user, tc.pass)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("GET / = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// TestWebCaptchaBlocksTheSocketWithoutSolvingIt: the actual enforcement
// point is /ws, not /play — a request that never loaded the challenge at
// all is refused before a session exists.
func TestWebCaptchaBlocksTheSocketWithoutSolvingIt(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := toWS(t, ts.URL) + "/ws"
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("dialing /ws with no captcha cookie succeeded, want a refusal")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		status := -1
		if resp != nil {
			status = resp.StatusCode
		}
		t.Errorf("status = %d, want %d", status, http.StatusForbidden)
	}
}

// TestWebCaptchaPlayShowsAChallengeUntilSolved: GET /play renders the
// arithmetic question, a wrong answer renders a new one rather than
// letting the old token through a second time, and a correct one sets the
// cookie /ws checks.
func TestWebCaptchaPlayShowsAChallengeUntilSolved(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", true)
	client := ts.Client()
	jar := newCookieJar(t)
	client.Jar = jar

	a, b, token := fetchCaptcha(t, client, ts.URL)

	// A wrong answer re-renders a challenge rather than erroring out, and
	// does not set the cleared cookie.
	resp, err := client.PostForm(ts.URL+"/play", url.Values{
		"token": {token}, "answer": {"not-a-number"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "wasn&#39;t it") {
		t.Errorf("wrong answer body = %q, want it to say so", body)
	}
	if len(jar.Cookies(mustParseURL(t, ts.URL))) != 0 {
		t.Error("a wrong answer set a cookie")
	}

	// The right answer redirects to /play and sets the cookie.
	resp2, err := client.PostForm(ts.URL+"/play", url.Values{
		"token": {token}, "answer": {itoa(a + b)},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.Request.URL.Path != "/play" {
		t.Errorf("ended up at %s, want /play", resp2.Request.URL.Path)
	}
	if len(jar.Cookies(mustParseURL(t, ts.URL))) == 0 {
		t.Fatal("a correct answer did not set the captcha cookie")
	}

	// And now GET /play renders the terminal, not another challenge.
	resp3, err := client.Get(ts.URL + "/play")
	if err != nil {
		t.Fatal(err)
	}
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	if strings.Contains(string(body3), "Before you go in") {
		t.Error("GET /play still showed the challenge after it was solved")
	}
	if !strings.Contains(string(body3), "xterm") {
		t.Errorf("GET /play after solving = %q, want the terminal page", body3)
	}
}

// TestWebSocketReachesTheRealLoginFlow is the end-to-end proof: /ws is not
// a different door into the game, it is the same one — the exact greeting
// text (the licence's own creator credit, obs.WithWizVisEcho's neighbour
// concern for a different licence entirely) arrives over the WebSocket
// with no telnet negotiation bytes ahead of it.
func TestWebSocketReachesTheRealLoginFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", nil)
	if err != nil {
		t.Fatalf("dialing /ws: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	conn := websocket.NetConn(ctx, c, websocket.MessageText)
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("reading the greeting: %v", err)
	}
	got := string(buf[:n])

	// The exact text this whole mechanism exists to guarantee never gets
	// skipped (session.go's own doc comment on Serve).
	if !strings.Contains(got, "Jeremy Elson") {
		t.Errorf("greeting = %q, want the CircleMUD creator credit", got)
	}
	if !strings.Contains(got, "By what name") {
		t.Errorf("greeting = %q, want the name prompt", got)
	}
	assertNoIAC(t, "greeting", got)

	// A step further: the name prompt is answered, which is what reaches
	// EchoOff — offer's own gate stops the login sequence's initial CHARSET/
	// GMCP negotiation, but the password prompt is a second, independent
	// place a telnet control sequence gets sent from (protocol.go), and it
	// needs its own proof.
	if _, err := conn.Write([]byte("Newcomer\r\n")); err != nil {
		t.Fatalf("sending a name: %v", err)
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("reading the name confirmation: %v", err)
	}
	if got := string(buf[:n]); !strings.Contains(got, "Y/N") {
		t.Fatalf("after a new name = %q, want the Y/N confirmation", got)
	}

	if _, err := conn.Write([]byte("y\r\n")); err != nil {
		t.Fatalf("confirming the name: %v", err)
	}
	// EchoOff's own marker (protocol.go's webEchoOffMarker) and the prompt
	// text that follows it are two separate SendRaw/Send calls from two
	// different call sites — session.go's own writeLoop has no reason to
	// coalesce them, and coder/websocket turns each Write into its own
	// WebSocket message — so they are read here as the two messages they
	// are rather than assumed to arrive in one.
	got = readUntilString(t, conn, "assword", buf)
	if !strings.Contains(got, "\ue000") {
		t.Errorf("before the password prompt = %q, want the echo-off marker", got)
	}
	assertNoIAC(t, "password prompt", got)
}

// readUntilString reads from conn until the accumulated text contains
// want, or the test's own context deadline gives out.
func readUntilString(t *testing.T, conn net.Conn, want string, buf []byte) string {
	t.Helper()
	var got strings.Builder
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			got.Write(buf[:n])
		}
		if strings.Contains(got.String(), want) {
			return got.String()
		}
		if err != nil {
			t.Fatalf("reading (want %q): %v; got so far: %q", want, err, got.String())
		}
	}
}

// assertNoIAC checks for a raw 0xFF byte — telnet's IAC — anywhere in got.
// strings.ContainsRune would not do: '\xff' as a rune literal is U+00FF,
// which is not the byte this is checking for at all once got is UTF-8 (its
// 2-byte encoding, 0xC3 0xBF, contains neither IAC nor anything close to
// it) — this needs a byte search, not a rune search.
func assertNoIAC(t *testing.T, what, got string) {
	t.Helper()
	if strings.IndexByte(got, 0xff) >= 0 {
		t.Errorf("%s contained a raw IAC byte: %q", what, got)
	}
}

// toWS turns an httptest server's http:// URL into a ws:// one.
func toWS(t *testing.T, httpURL string) string {
	t.Helper()
	u, err := url.Parse(httpURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Scheme = "ws"
	return u.String()
}

var questionRe = regexp.MustCompile(`What is (\d+) &#43; (\d+)`)
var tokenRe = regexp.MustCompile(`name="token" value="([^"]+)"`)

// fetchCaptcha GETs /play and pulls the two operands and the signed token
// out of the challenge it renders.
func fetchCaptcha(t *testing.T, client *http.Client, baseURL string) (a, b int, token string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/play")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	q := questionRe.FindStringSubmatch(string(body))
	tok := tokenRe.FindStringSubmatch(string(body))
	if q == nil || tok == nil {
		t.Fatalf("GET /play = %q, want a captcha challenge", body)
	}
	return atoi(t, q[1]), atoi(t, q[2]), tok[1]
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// newCookieJar is a plain http.CookieJar, factored out only so the two
// call sites that need one (the client and the assertion reading it back)
// share the same instance.
func newCookieJar(t *testing.T) *testJar {
	t.Helper()
	return &testJar{store: map[string][]*http.Cookie{}}
}

// testJar is the smallest possible http.CookieJar: net/http/cookiejar
// would work too, but this is enough for one host and keeps the test's
// own assertions (len(jar.Cookies(u))) reading directly off what was set,
// without cookiejar's own expiry/domain-matching rules in between.
type testJar struct {
	store map[string][]*http.Cookie
}

func (j *testJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.store[u.Host] = cookies
}

func (j *testJar) Cookies(u *url.URL) []*http.Cookie {
	return j.store[u.Host]
}
