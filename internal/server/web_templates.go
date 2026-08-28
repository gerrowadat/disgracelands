// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import "html/template"

// The three pages the web interface serves. Styling is inline and
// deliberately minimal — a dark background and a monospace font are what
// "looks like a telnet session" actually requires outside the terminal
// itself, which is xterm.js's job (playTemplate) and gets its look from
// the server's own ANSI colour codes, the same ones a real telnet client
// renders.
//
// xterm.js is loaded from a CDN rather than vendored: nothing in the Go
// binary or this repository's own dependency graph changes because of it,
// which is the tradeoff — a browser that cannot reach jsdelivr cannot open
// /play, exactly as one that cannot reach any other CDN a self-hosted page
// depends on cannot. See docs/deviations.md.

var pageStyle = `
	:root { color-scheme: dark; }
	html, body {
		margin: 0; height: 100%;
		background: #0b0f0c; color: #c8ffc8;
		font-family: "Courier New", ui-monospace, Consolas, monospace;
	}
	a { color: #6fffb0; }
	.box {
		max-width: 640px; margin: 4rem auto; padding: 2rem;
		background: #10160f; border: 1px solid #234a2f;
	}
	h1 { color: #9fffb0; font-weight: normal; letter-spacing: 0.05em; }
	.button {
		display: inline-block; margin-top: 1rem;
		background: #14231a; color: #c8ffc8; border: 1px solid #4a8a5f;
		padding: 0.6rem 1.2rem; text-decoration: none; font-family: inherit;
	}
	.button:hover { background: #1c3324; }
	input[type=text] {
		background: #0b0f0c; color: #c8ffc8; border: 1px solid #4a8a5f;
		padding: 0.4rem; font-family: inherit; font-size: 1rem;
	}
	.error { color: #ff8080; }
`

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html><head><meta charset="utf-8">
<title>Disgracelands</title>
<style>` + pageStyle + `</style>
</head><body>
<div class="box">
<h1>Disgracelands</h1>
<p>A CircleMUD, played 2001&ndash;2008 and ported here to keep playing.</p>
<p><a class="button" href="/play">Play</a></p>
</div>
</body></html>
`))

var captchaTemplate = template.Must(template.New("captcha").Parse(`<!doctype html>
<html><head><meta charset="utf-8">
<title>Disgracelands &mdash; one moment</title>
<style>` + pageStyle + `</style>
</head><body>
<div class="box">
<h1>Before you go in</h1>
<p>What is {{.Question}}?</p>
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
<form method="post" action="/play">
<input type="hidden" name="token" value="{{.Token}}">
<input type="text" name="answer" autofocus autocomplete="off">
<button class="button" type="submit">Continue</button>
</form>
</div>
</body></html>
`))

// playTemplate is the terminal itself. term.onData sends every keystroke
// to /ws as it comes rather than batching a line at a time — the same
// character-at-a-time behaviour a real telnet client gives the server,
// which is what lets the pager answer a single keypress without Enter.
//
// Echoing what is typed is this page's own job, and not a small one:
// xterm.js has no local echo at all, unlike a real telnet client's
// terminal driver, which echoes by itself and only stops when told to
// around a password. See the script's own comment on localEcho for how
// that gap is closed — session.go's webEchoOff/OnMarker is the other half
// of it.
var playTemplate = template.Must(template.New("play").Parse(`<!doctype html>
<html><head><meta charset="utf-8">
<title>Disgracelands</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css">
<script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.js"></script>
<style>
	html, body { margin: 0; height: 100%; background: #0b0f0c; }
	#terminal { height: 100vh; width: 100vw; }
</style>
</head><body>
<div id="terminal"></div>
<script>
(function () {
	var term = new Terminal({
		fontFamily: '"Courier New", ui-monospace, Consolas, monospace',
		fontSize: 15,
		theme: { background: '#0b0f0c', foreground: '#c8ffc8', cursor: '#c8ffc8' },
	});
	var fit = new FitAddon.FitAddon();
	term.loadAddon(fit);
	term.open(document.getElementById('terminal'));
	fit.fit();
	window.addEventListener('resize', function () { fit.fit(); });

	// Whether to echo locally, toggled by the two markers session.go's
	// EchoOff/EchoOn send in place of a telnet IAC WILL/WONT ECHO (there
	// is no telnet client here to negotiate ECHO with) — U+E000 to turn
	// it off around a password, U+E001 to turn it back on. Both are
	// Unicode Private Use Area code points with no meaning of their own,
	// so they cannot collide with anything the game actually sends, and
	// both are stripped here before the rest of the message reaches the
	// terminal.
	var localEcho = true;

	var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	var ws = new WebSocket(proto + '//' + location.host + '/ws');
	ws.onmessage = function (ev) {
		var data = ev.data;
		if (data.indexOf('\uE000') !== -1) {
			localEcho = false;
			data = data.split('\uE000').join('');
		}
		if (data.indexOf('\uE001') !== -1) {
			localEcho = true;
			data = data.split('\uE001').join('');
		}
		term.write(data);
	};
	ws.onopen = function () { term.focus(); };
	ws.onclose = function () { term.write('\r\n\x1b[31m[connection closed]\x1b[0m\r\n'); };
	ws.onerror = function () { term.write('\r\n\x1b[31m[connection error]\x1b[0m\r\n'); };
	term.onData(function (data) {
		if (ws.readyState === WebSocket.OPEN) ws.send(data);
		if (!localEcho) return;
		// Minimal local line editing: echo what was typed, and let
		// backspace erase it back off the screen — matching what a real
		// telnet client's own terminal driver already does before a line
		// ever reaches the server. \r\n is collapsed to \r first so a
		// pasted chunk with Windows line endings does not echo a blank
		// line for every one of them.
		data = data.replace(/\r\n/g, '\r');
		for (var i = 0; i < data.length; i++) {
			var ch = data[i];
			if (ch === '\r' || ch === '\n') {
				term.write('\r\n');
			} else if (ch === '\x7f' || ch === '\b') {
				term.write('\b \b');
			} else {
				term.write(ch);
			}
		}
	});
})();
</script>
</body></html>
`))
