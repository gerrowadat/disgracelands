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
// which is what lets password entry suppress local echo and the pager
// answer a single keypress without Enter.
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

	var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	var ws = new WebSocket(proto + '//' + location.host + '/ws');
	ws.onmessage = function (ev) { term.write(ev.data); };
	ws.onopen = function () { term.focus(); };
	ws.onclose = function () { term.write('\r\n\x1b[31m[connection closed]\x1b[0m\r\n'); };
	ws.onerror = function () { term.write('\r\n\x1b[31m[connection error]\x1b[0m\r\n'); };
	term.onData(function (data) {
		if (ws.readyState === WebSocket.OPEN) ws.send(data);
	});
})();
</script>
</body></html>
`))
