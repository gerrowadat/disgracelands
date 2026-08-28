// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html/template"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/gerrowadat/disgracelands/internal/session"
)

// The web interface: a welcome page at /, a browser terminal at /play, and
// the WebSocket upgrade at /ws that terminal actually speaks over — the
// browser client docs/configuration.md used to say --listen-ws had none of.
//
// It is not a new front door onto a different game: /ws is wired straight
// into [Server.serve], the exact function every telnet connection already
// goes through, by wrapping the upgraded connection as a net.Conn
// ([websocket.NetConn]) and handing it the same session, login and
// shutdown machinery. A player who reaches the game over the web sees the
// same name prompt, the same MOTD, the same everything — the only new
// thing is what carries the bytes, and xterm.js in the browser renders the
// server's own ANSI colour codes directly, which is what makes a browser
// tab "look like a telnet session" without the server treating it as
// anything other than one.
//
// Two things gate access, both optional and both off by default:
//
//   - WebPassword, if set, is HTTP Basic Auth in front of every route —
//     the web interface's own front door, on top of the game's login
//     prompt behind it, not instead of it. Any username is accepted; only
//     the password is checked, because this is one shared secret for
//     "may use the web interface at all", not a second account system.
//   - WebCaptcha, if set, requires solving a trivial arithmetic question
//     before /ws will upgrade a connection. This is not meant to defeat a
//     determined attacker — the answer space is small enough to brute
//     force in seconds — it exists to raise the cost of "point a script
//     at the web port" above "point a script at the telnet port", which
//     is the actual, modest threat model asked for.

// webHandler holds what the web interface's routes need beyond *Server
// itself: its own per-boot signing key (so a token survives nothing longer
// than the process that minted it), and its own admission-control state,
// mirroring Accept's own perHost/nextID locals — see admitConn's doc
// comment for why this is a deliberate duplication rather than a shared
// helper.
type webHandler struct {
	s        *Server
	ctx      context.Context //nolint:containedctx // bounds every WebSocket net.Conn's lifetime; see [websocket.NetConn]'s own doc.
	limits   Limits
	password string
	captcha  bool

	secret [32]byte

	nextID  atomic.Uint64
	perHost sync.Map
}

// WebHandler builds the web interface's http.Handler: / (welcome), /play
// (the browser terminal, or a captcha challenge in front of it), and /ws
// (the WebSocket upgrade /play's terminal actually connects to). ctx bounds
// every session opened through it, exactly as the ctx passed to Accept
// bounds every telnet one, so a server shutdown reaches web players too.
func (s *Server) WebHandler(ctx context.Context, password string, captcha bool, limits Limits) (http.Handler, error) {
	h := &webHandler{s: s, ctx: ctx, limits: limits, password: password, captcha: captcha}
	if _, err := rand.Read(h.secret[:]); err != nil {
		return nil, fmt.Errorf("generating the web interface's signing key: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("GET /play", h.handlePlay)
	mux.HandleFunc("POST /play", h.handleCaptchaSubmit)
	mux.HandleFunc("GET /ws", h.handleWS)

	var handler http.Handler = mux
	if password != "" {
		handler = h.requirePassword(handler)
	}
	return handler, nil
}

// requirePassword is the whole web interface's own front door: HTTP Basic
// Auth against one shared password, checked in constant time. The
// username is not checked at all — WWW-Authenticate still asks for one,
// because a browser's own login dialog expects the pair, but there is
// nothing behind it to be a name for.
func (h *webHandler) requirePassword(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(h.password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Disgracelands"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleIndex is the welcome page: what a browser sees at the web
// interface's own root, before it has anything to do with a character at
// all.
func (h *webHandler) handleIndex(w http.ResponseWriter, r *http.Request) {
	renderPage(w, indexTemplate, nil)
}

// handlePlay is /play: the browser terminal itself, or — if WebCaptcha is
// on and this browser has not solved one recently — the challenge that
// stands in front of it. The terminal page never talks to the game
// directly; it only knows how to open a WebSocket to /ws, which is where
// the captcha cookie is actually enforced (see handleWS) — this page's own
// check is what saves a browser a redirect, not what a script could not
// simply skip.
func (h *webHandler) handlePlay(w http.ResponseWriter, r *http.Request) {
	if h.captcha && !h.hasCleared(r) {
		h.renderCaptcha(w, "")
		return
	}
	renderPage(w, playTemplate, nil)
}

// handleCaptchaSubmit is POST /play: an answer to the challenge
// handlePlay rendered. A correct, unexpired answer mints the "cleared"
// cookie handleWS checks and redirects back to GET /play, which now
// serves the terminal; anything else re-renders a fresh challenge — the
// old token is single-use by construction, since a new one is issued every
// time regardless of why the old one failed.
func (h *webHandler) handleCaptchaSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.captcha {
		http.Redirect(w, r, "/play", http.StatusSeeOther)
		return
	}
	if !h.verifyToken(r.FormValue("token"), "captcha", r.FormValue("answer")) {
		h.renderCaptcha(w, "That wasn't it. Try again.")
		return
	}

	// Secure is conditional on r.TLS, which is nil behind a
	// TLS-terminating reverse proxy — the same deployment
	// Config.Warnings already calls out for --listen-ws itself:
	// --trust-proxy-headers exists but nothing reads X-Forwarded-Proto
	// yet, so this cookie is Secure only when this process terminates
	// the TLS itself.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is deliberately conditional; see the comment above
		Name:     captchaCookie,
		Value:    h.signToken("cleared", "", clearedTTL),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(clearedTTL.Seconds()),
	})
	http.Redirect(w, r, "/play", http.StatusSeeOther)
}

// renderCaptcha shows the arithmetic challenge, with problem an optional
// error line from a previous, wrong answer.
func (h *webHandler) renderCaptcha(w http.ResponseWriter, problem string) {
	a, b := randSmallInt(), randSmallInt()
	renderPage(w, captchaTemplate, captchaPage{
		Question: fmt.Sprintf("%d + %d", a, b),
		Token:    h.signToken("captcha", strconv.Itoa(a+b), captchaTTL),
		Error:    problem,
	})
}

// hasCleared reports whether r already carries a valid "cleared" cookie —
// GET /play's own shortcut around showing a challenge it already knows the
// answer to.
func (h *webHandler) hasCleared(r *http.Request) bool {
	c, err := r.Cookie(captchaCookie)
	if err != nil {
		return false
	}
	return h.verifyToken(c.Value, "cleared", "")
}

// handleWS is /ws: the WebSocket upgrade the terminal page opens, and the
// actual enforcement point for WebCaptcha — a script that never loaded
// /play at all, and so never solved anything, arrives here with no cookie
// and is refused before a session ever exists. Everything past the
// upgrade is ordinary [Server.serve]: the same session, the same login
// prompt, the same shutdown handling telnet gets, over a net.Conn that
// happens to be a WebSocket underneath.
func (h *webHandler) handleWS(w http.ResponseWriter, r *http.Request) {
	if h.captcha && !h.hasCleared(r) {
		http.Error(w, "solve the captcha at /play first", http.StatusForbidden)
		return
	}

	wc, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has already written its own response.
	}
	conn := websocket.NetConn(h.ctx, wc, websocket.MessageText)

	hostKey, ok := h.admitConn(conn)
	if !ok {
		return // admitConn has already told the browser why and closed conn.
	}
	defer func() {
		if c, ok := h.perHost.Load(hostKey); ok {
			c.(*atomic.Int64).Add(-1) //nolint:forcetypeassert // stored as *atomic.Int64 two lines above
		}
	}()

	sess := session.New(h.nextID.Add(1), conn, "websocket", h.s.logger)
	h.s.serve(h.ctx, sess, h.limits)
}

// admitConn is Accept's own admission checks (listen.go: server-full,
// too-many-from-one-host), duplicated rather than shared. Accept's version
// lives inside one accept loop's closure and decrements its counter
// through the *atomic.Int64 that loop already has in scope; handleWS has
// no loop, one connection at a time, arriving from net/http rather than a
// net.Listener.Accept, and no clean way to share that closure without
// changing Accept's own signature for a code path it does not otherwise
// need to know about. The checks themselves — what "server full" and "too
// many from one host" mean — are the part that has to agree, and this
// keeps them.
func (h *webHandler) admitConn(conn net.Conn) (hostKey string, ok bool) {
	if h.limits.MaxPlayers > 0 && h.s.connections.count() >= h.limits.MaxPlayers {
		h.s.logger.Warn("refusing a web connection: server full", "limit", h.limits.MaxPlayers)
		_, _ = conn.Write([]byte("Sorry, CircleMUD is full right now... please try again later!\r\n"))
		_ = conn.Close()
		return "", false
	}

	host, _, splitErr := net.SplitHostPort(conn.RemoteAddr().String())
	if splitErr != nil {
		host = conn.RemoteAddr().String()
	}
	hostKey = perHostKey(host)

	countAny, _ := h.perHost.LoadOrStore(hostKey, new(atomic.Int64))
	count, _ := countAny.(*atomic.Int64) //nolint:forcetypeassert // this map only ever stores *atomic.Int64
	if h.limits.MaxPerHost > 0 && count.Load() >= int64(h.limits.MaxPerHost) {
		h.s.logger.Warn("refusing a web connection: too many from this address",
			"host", host, "limit", h.limits.MaxPerHost)
		_, _ = conn.Write([]byte("Too many connections from your address.\r\n"))
		_ = conn.Close()
		return "", false
	}
	count.Add(1)
	return hostKey, true
}

// Signed tokens: a captcha's answer, and the "already solved one" cookie,
// are both HMAC-signed "kind.payload.expiry" strings under the handler's
// own per-boot secret — nothing is kept server-side, so there is no store
// to clean up and no state that survives a restart. kind stops a captcha
// token being replayed as a cleared cookie or the reverse; payload must
// never itself contain a ".", which both current payloads (an integer
// answer, or empty) satisfy by construction.
const (
	captchaCookie = "dl_captcha"
	captchaTTL    = 5 * time.Minute
	clearedTTL    = 30 * time.Minute
)

func (h *webHandler) signToken(kind, payload string, ttl time.Duration) string {
	body := fmt.Sprintf("%s.%s.%d", kind, payload, time.Now().Add(ttl).Unix())
	mac := hmac.New(sha256.New, h.secret[:])
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString([]byte(body)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyToken checks a token minted by signToken: right kind, correct
// signature, not expired, and — when wantAnswer is non-empty — that the
// token's own payload matches it. Every failure returns false uniformly;
// a token that fails to even parse is indistinguishable from one whose
// signature is wrong.
func (h *webHandler) verifyToken(token, kind, wantAnswer string) bool {
	outer := strings.SplitN(token, ".", 2)
	if len(outer) != 2 {
		return false
	}
	body, err := base64.RawURLEncoding.DecodeString(outer[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(outer[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.secret[:])
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return false
	}

	fields := strings.SplitN(string(body), ".", 3)
	if len(fields) != 3 || fields[0] != kind {
		return false
	}
	exp, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return wantAnswer == "" || fields[1] == wantAnswer
}

// randSmallInt is one operand of the captcha's question — small enough
// that the sum stays a short, easy piece of mental arithmetic. This is
// deliberately not internal/rng: that package's whole reason to exist is
// reproducing the C server's own dice against oracles, which a web page's
// arithmetic puzzle has nothing to do with, and crypto/rand is what is
// already in this file for the signing key.
func randSmallInt() int {
	n, err := rand.Int(rand.Reader, big.NewInt(20))
	if err != nil {
		return 7 // rand.Reader failing is a broken machine, not a broken captcha.
	}
	return int(n.Int64()) + 1
}

// renderPage executes a parsed template straight to w, matching every
// other plain-text response in this file: a template error is a
// programmer error (a bad template, not bad input), so it is logged
// nowhere special and simply fails the request the ordinary net/http way.
func renderPage(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(w, data)
}

type captchaPage struct {
	Question string
	Token    string
	Error    string
}
