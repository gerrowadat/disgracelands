// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestForwardedFor is the header parsing on its own: which element wins,
// and which spellings of an address are read.
func TestForwardedFor(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string // "" for "nothing usable"
	}{
		{"empty", nil, ""},
		{"blank", []string{""}, ""},
		{"one", []string{"203.0.113.7"}, "203.0.113.7"},
		{"spaces", []string{"  203.0.113.7  "}, "203.0.113.7"},

		// The security case. A client forging a first entry gets its own
		// address appended by the proxy, so the rightmost is the real one.
		{"forged first entry", []string{"1.2.3.4, 203.0.113.7"}, "203.0.113.7"},
		// net/http keeps repeated headers as separate values; the same rule
		// has to hold across them, not just within one.
		{"repeated header", []string{"1.2.3.4", "203.0.113.7"}, "203.0.113.7"},

		// Spellings proxies actually write.
		{"with a port", []string{"203.0.113.7:41234"}, "203.0.113.7"},
		{"ipv6", []string{"2001:db8::1"}, "2001:db8::1"},
		{"ipv6 bracketed with a port", []string{"[2001:db8::1]:41234"}, "2001:db8::1"},

		// Unmapped, because a ban matches on the host string and
		// ::ffff:203.0.113.7 would not match a ban on 203.0.113.7.
		{"ipv4-mapped", []string{"::ffff:203.0.113.7"}, "203.0.113.7"},
		// A scope identifier means nothing off the machine that owns the
		// interface, and would otherwise make two spellings of one address.
		{"zoned", []string{"fe80::1%eth0"}, "fe80::1"},

		// Rubbish is skipped rather than trusted, and the search keeps
		// walking left past it.
		{"not an address", []string{"unknown"}, ""},
		{"obfuscated node", []string{"_hidden"}, ""},
		{"rubbish last, address first", []string{"203.0.113.7, unknown"}, "203.0.113.7"},
		{"a hostname", []string{"proxy.example.com"}, ""},
		{"empty last element", []string{"203.0.113.7, "}, "203.0.113.7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for _, v := range tc.values {
				h.Add(headerForwardedFor, v)
			}
			got, ok := forwardedFor(h)
			if tc.want == "" {
				if ok {
					t.Errorf("forwardedFor(%q) = %v, want nothing usable", tc.values, got)
				}
				return
			}
			if !ok {
				t.Fatalf("forwardedFor(%q) found nothing, want %s", tc.values, tc.want)
			}
			if got.String() != tc.want {
				t.Errorf("forwardedFor(%q) = %s, want %s", tc.values, got, tc.want)
			}
		})
	}
}

// TestForwardedHTTPS is the other header, which decides whether the
// captcha cookie is Secure.
func TestForwardedHTTPS(t *testing.T) {
	cases := []struct {
		values []string
		want   bool
	}{
		{nil, false},
		{[]string{"https"}, true},
		{[]string{"HTTPS"}, true}, // the scheme is case-insensitive
		{[]string{"http"}, false},
		{[]string{"ws"}, false},
		// Rightmost, as with X-Forwarded-For: the nearest hop is the one
		// this process can trust something about.
		{[]string{"https, http"}, false},
		{[]string{"http, https"}, true},
		{[]string{"http", "https"}, true},
	}
	for _, tc := range cases {
		h := http.Header{}
		for _, v := range tc.values {
			h.Add(headerForwardedProto, v)
		}
		if got := forwardedHTTPS(h); got != tc.want {
			t.Errorf("forwardedHTTPS(%q) = %v, want %v", tc.values, got, tc.want)
		}
	}
}

// listeningWebProxied is listeningWeb with --trust-proxy-headers on.
func listeningWebProxied(t *testing.T, srv *Server, limits Limits) *httptest.Server {
	t.Helper()
	h, err := srv.WebHandler(context.Background(), WebOptions{
		TrustProxyHeaders: true, Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

// dialWS opens /ws with the given headers and returns the greeting.
func dialWS(t *testing.T, ts *httptest.Server, header http.Header) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dialing /ws: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(websocket.StatusNormalClosure, "") })

	conn := websocket.NetConn(ctx, c, websocket.MessageText)
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("reading the greeting: %v", err)
	}
	// The name prompt is where the ban is checked (login.go, matching the
	// C's CON_GET_NAME), so a name has to be typed before a refusal can
	// arrive.
	if _, err := conn.Write([]byte("Hopeful\r\n")); err != nil {
		t.Fatalf("sending a name: %v", err)
	}
	greeting := string(buf[:n])
	m, err := conn.Read(buf)
	if err != nil {
		return greeting
	}
	return greeting + string(buf[:m])
}

// TestAWebPlayerIsBannedByTheirForwardedAddress is the end of the chain
// this flag exists for: a header, resolved once at the upgrade, reaching
// the ban check through Session.Host().
//
// Bans are the sharpest of the four things the address feeds, because
// getting it wrong is wrong in both directions at once — behind a proxy,
// `ban site` cannot reach a web player at all, and banning the proxy locks
// out every web player there is.
func TestAWebPlayerIsBannedByTheirForwardedAddress(t *testing.T) {
	srv, _ := newTestServer(t)

	// Ban one address a web player will claim to be at, over telnet, using
	// the ordinary in-game command.
	god := dialClient(t, listening(t, srv))
	god.create("Warden", "nobodycomesin", "m", "w")
	god.send("ban all 203.0.113.7")
	god.expect("Site banned.")

	ts := listeningWebProxied(t, srv, webTestLimits)

	banned := http.Header{}
	banned.Set(headerForwardedFor, "203.0.113.7")
	if got := dialWS(t, ts, banned); !strings.Contains(got, "You are not welcome here.") {
		t.Errorf("a banned forwarded address got in; the transcript was:\n%s", got)
	}

	// And the proxy's own address — the loopback, which is what the socket
	// actually reports — is not banned, so somebody else behind the same
	// proxy still gets in. Without the header being read, this connection
	// and the one above are indistinguishable.
	allowed := http.Header{}
	allowed.Set(headerForwardedFor, "198.51.100.4")
	if got := dialWS(t, ts, allowed); strings.Contains(got, "You are not welcome here.") {
		t.Errorf("an unbanned address behind the same proxy was refused:\n%s", got)
	}
}

// TestTheForwardedAddressIsIgnoredByDefault. Off is the only safe default:
// with nothing in front of this process, the header is whatever the client
// typed, so believing it would be a free way past a site ban.
func TestTheForwardedAddressIsIgnoredByDefault(t *testing.T) {
	srv, _ := newTestServer(t)

	god := dialClient(t, listening(t, srv))
	god.create("Warden", "nobodycomesin", "m", "w")
	// Ban the loopback: the address the socket really comes from.
	god.send("ban all 127.0.0.1")
	god.expect("Site banned.")

	ts := listeningWeb(t, srv, "", false)

	forged := http.Header{}
	forged.Set(headerForwardedFor, "203.0.113.7")
	if got := dialWS(t, ts, forged); !strings.Contains(got, "You are not welcome here.") {
		t.Errorf("a forged X-Forwarded-For evaded a ban with the flag off:\n%s", got)
	}
}

// TestForwardedAddressesGetTheirOwnPerHostBucket. The other half of what
// the inert flag cost: with every web connection reporting the proxy's
// address, --max-connections-per-ip caps the entire web interface rather
// than one player, and its default is 8.
func TestForwardedAddressesGetTheirOwnPerHostBucket(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWebProxied(t, srv, Limits{MaxPerHost: 1, LoginGrace: time.Minute})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dial := func(claimed string) *websocket.Conn {
		t.Helper()
		header := http.Header{}
		header.Set(headerForwardedFor, claimed)
		c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", &websocket.DialOptions{HTTPHeader: header})
		if err != nil {
			t.Fatalf("dialing /ws as %s: %v", claimed, err)
		}
		return c
	}

	first := dial("203.0.113.7")
	defer func() { _ = first.Close(websocket.StatusNormalClosure, "") }()
	// The greeting has to be read before the count can be relied on: the
	// upgrade returns as soon as the handshake is done, and admitConn runs
	// on the handler's own goroutine afterwards.
	firstConn := websocket.NetConn(ctx, first, websocket.MessageText)
	buf := make([]byte, 8192)
	if _, err := firstConn.Read(buf); err != nil {
		t.Fatalf("reading the first greeting: %v", err)
	}

	// A different claimed address is a different bucket, so it is admitted
	// even at MaxPerHost 1. Both would share the loopback's bucket without
	// the header being read, and this one would be refused.
	second := dial("198.51.100.4")
	defer func() { _ = second.Close(websocket.StatusNormalClosure, "") }()
	secondConn := websocket.NetConn(ctx, second, websocket.MessageText)
	n, err := secondConn.Read(buf)
	if err != nil {
		t.Fatalf("reading the second greeting: %v", err)
	}
	if got := string(buf[:n]); strings.Contains(got, "Too many connections") {
		t.Errorf("a second, different address was refused: %q", got)
	}

	// And the same claimed address twice is refused, which is the limit
	// still doing its job rather than having been turned off.
	third := dial("203.0.113.7")
	defer func() { _ = third.Close(websocket.StatusNormalClosure, "") }()
	thirdConn := websocket.NetConn(ctx, third, websocket.MessageText)
	n, err = thirdConn.Read(buf)
	if err != nil {
		t.Fatalf("reading the third response: %v", err)
	}
	if got := string(buf[:n]); !strings.Contains(got, "Too many connections") {
		t.Errorf("a second connection from one address was admitted: %q", got)
	}
}

// TestTheClearedCookieIsSecureBehindATLSTerminatingProxy is the other
// header's job.
//
// The captcha cookie is what /ws checks, and behind a TLS-terminating
// proxy r.TLS is nil however the browser got there — so without
// X-Forwarded-Proto the cookie travelled without Secure over a hop the
// operator believes is HTTPS, which is precisely the deployment
// Config.Warnings already calls out for --listen-ws itself.
func TestTheClearedCookieIsSecureBehindATLSTerminatingProxy(t *testing.T) {
	srv, _ := newTestServer(t)

	solve := func(ts *httptest.Server, proto string) *http.Cookie {
		t.Helper()
		client := ts.Client()
		// No jar: the Set-Cookie header itself is what is under test, and a
		// jar would keep the attributes to itself.
		a, b, token := fetchCaptcha(t, client, ts.URL)

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/play",
			strings.NewReader(url.Values{"token": {token}, "answer": {itoa(a + b)}}.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if proto != "" {
			req.Header.Set(headerForwardedProto, proto)
		}
		// The redirect is not followed: it is this response's own
		// Set-Cookie that carries the attributes.
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		for _, c := range resp.Cookies() {
			if c.Name == captchaCookie {
				return c
			}
		}
		t.Fatalf("no %s cookie in the response (status %d)", captchaCookie, resp.StatusCode)
		return nil
	}

	proxied := listeningWebProxiedCaptcha(t, srv)
	if got := solve(proxied, "https"); !got.Secure {
		t.Error("X-Forwarded-Proto: https did not make the cleared cookie Secure")
	}
	// Plain http through the same proxy must not: a Secure cookie on a
	// genuinely plaintext hop is never sent back, and the captcha becomes a
	// loop with no error message anywhere.
	if got := solve(proxied, "http"); got.Secure {
		t.Error("X-Forwarded-Proto: http made the cookie Secure")
	}

	// And with the flag off the header is ignored, as everywhere else.
	plain := listeningWeb(t, srv, "", true)
	if got := solve(plain, "https"); got.Secure {
		t.Error("X-Forwarded-Proto was believed with --trust-proxy-headers off")
	}
}

// listeningWebProxiedCaptcha is listeningWebProxied with the captcha on,
// since that is the only thing that mints the cookie.
func listeningWebProxiedCaptcha(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	h, err := srv.WebHandler(context.Background(), WebOptions{
		Captcha: true, TrustProxyHeaders: true, Limits: webTestLimits,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}
