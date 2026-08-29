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

	// The arrow keys, and the one of them that does something.
	//
	// xterm.js delivers a cursor key as its own onData call, as an escape
	// sequence: ESC [ A-D normally, ESC O A-D when an application has put
	// the terminal into cursor-key mode (DECCKM). The game understands
	// neither, so before this they were forwarded to the server as command
	// text -- ESC, '[' and a letter, which is why an arrow key at the name
	// prompt answered "Names may only contain letters." -- and echoed back
	// into the terminal, where xterm read them again and moved the cursor.
	// Neither happens now: an arrow key is swallowed here and never
	// reaches either.
	var ARROW = {
		'\x1b[A': 'up', '\x1b[B': 'down', '\x1b[C': 'right', '\x1b[D': 'left',
		'\x1bOA': 'up', '\x1bOB': 'down', '\x1bOC': 'right', '\x1bOD': 'left',
	};

	// What the player has typed on the line so far, as they can see it,
	// and the last line they finished. lastCommand is what up-arrow
	// repeats.
	//
	// 'line' is also what the server holds, exactly: every keystroke goes
	// out as it is typed (see term.onData below; the pager depends on it),
	// and since #233 the server erases on a backspace the same way this
	// does. Before that fix the two diverged -- a backspace was erased on
	// screen and still counted by the server -- and up-arrow had to track
	// the difference separately to know whether it was safe to type.
	var line = '';
	var lastCommand = '';

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
	// send types a line at the game as if the player had: the text, then
	// the Enter that runs it. Used by the up-arrow repeat.
	function send(text) {
		if (ws.readyState !== WebSocket.OPEN) return;
		ws.send(text + '\r');
		consume(text + '\r', localEcho);
	}

	// repeat is the up-arrow: run the last command again.
	//
	// Two things stop it. The line must be empty: a repeat is sent as
	// text plus an Enter, so with a half-typed line already in the
	// server's buffer the two would run together -- "ki" plus a repeated
	// "look" is the command "kilook". Erasing back to an empty line is
	// enough now that the server erases too, which it was not before
	// #233. And local echo must be on: echo is off around a password, so
	// a password is never recorded as lastCommand and up-arrow can never
	// replay one in the clear.
	function repeat() {
		if (!lastCommand || line !== '' || !localEcho) return;
		send(lastCommand);
	}

	// consume walks what was typed, keeping 'line' and 'lastCommand' up to
	// date, and echoing it if echo is on.
	//
	// Echoing is this page's own job: xterm.js has no local echo, unlike a
	// real telnet client's terminal driver, which echoes by itself and
	// only stops when told to around a password. Backspace is erased off
	// the screen the same way that driver would, and the byte still goes
	// to the server -- which since #233 erases on it as well, so the two
	// buffers stay in step. readLoop's own comment has the C citation.
	//
	// \r\n is collapsed to \r first so a pasted chunk with Windows line
	// endings does not echo a blank line, or record an empty command, for
	// every one of them.
	function consume(data, echo) {
		data = data.replace(/\r\n/g, '\r');
		for (var i = 0; i < data.length; i++) {
			var ch = data[i];
			if (ch === '\r' || ch === '\n') {
				if (echo) term.write('\r\n');
				// Only a line the player could actually see becomes the
				// one up-arrow repeats.
				if (echo && line !== '') lastCommand = line;
				line = '';
			} else if (ch === '\x7f' || ch === '\b') {
				if (echo) term.write('\b \b');
				line = line.slice(0, -1);
			} else {
				if (echo) term.write(ch);
				line += ch;
			}
		}
	}

	term.onData(function (data) {
		// Arrow keys never reach the server and are never echoed. Only a
		// whole keystroke that *is* one counts, so a paste that happens
		// to contain an escape sequence is still forwarded whole, as it
		// was before. The ESC test comes first so that the lookup cannot
		// reach Object.prototype -- without it, pasting the literal word
		// "constructor" finds a function there, and would be swallowed.
		var arrow = data.charCodeAt(0) === 27 ? ARROW[data] : undefined;
		if (arrow !== undefined) {
			if (arrow === 'up') repeat();
			return;
		}
		if (ws.readyState === WebSocket.OPEN) ws.send(data);
		consume(data, localEcho);
	});
})();
</script>
</body></html>
`))
