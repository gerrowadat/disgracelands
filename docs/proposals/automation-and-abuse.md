# Automation and abuse: raising the cost of bots, surviving a flood

> **Status, 2026-08-30: proposed, nothing built.** This is a plan, not a
> record of one. Nothing from §4 onwards exists; §2 is an inventory of
> what already does.
>
> It is written now because of what v1.0.0 changed about the question.
> `TODO.md`'s open item 4 — hosting and exposure — says "nothing here has
> been hardened for 2026", and `docs/operations.md`'s "Exposure" section
> answers the whole thing with *don't*: TLS only, loopback or VPN, local.
> That is the correct posture for a server nobody plays and it stops being
> an answer on the day one does. This document is what would have to exist
> before that answer can change.
>
> It is **downstream of a decision nobody has made** — `TODO.md`'s item 4
> — and it is scoped accordingly: not a forward plan, not a sequence of
> work anybody is committing to, and §10 is ordered so that its first
> three rows stand on their own if the rest is never built.

Two things were asked for and they are treated together, because the
useful part of the answer is the same for both: **make automation cost
something, gather evidence, and act on scale rather than on individuals.**

1. Stop people pointing an LLM at the game and having it play — or at
   least make it non-trivial.
2. Detect and prevent denial of service and brigading, automatically.

The first of those cannot be done and the document says so in §3 rather
than at the end. What can be done is worth doing anyway, and the shape it
takes is not the shape the phrase "bot detection" suggests.

---

## 0. Decisions taken

Assumed by everything below; argue with these first if you are going to
argue.

1. **No automated punishment.** Nothing in this proposal bans, freezes,
   deletes or reduces a character on its own. Automatic responses are
   limited to *reversible, non-punitive* measures — slowing something
   down, refusing a new connection, declining new logins for a while.
   Anything that costs a player something is an immortal's decision, made
   with evidence in front of them. §7.4 has the reasoning; it is mostly
   about IPv6.

2. **A false positive is worse than a false negative.** This is a
   twenty-five-year-old game with a small, returning player base. Wrongly
   accusing one real player of botting costs more than a hundred bots
   gain. Every threshold in here is set on that basis, and every signal is
   advisory.

3. **Accessibility is a constraint, not a trade-off.** Nothing may
   require sight, reaction time, motor precision, or a particular client.
   That rules out several standard anti-bot techniques outright, and it
   rules them out *first* rather than as an afterthought — see §4.5. The
   repository has an `accessibility` label for a reason.

4. **No behavioural surveillance retained beyond its use.** The signals in
   §5 are computed over a rolling window in memory and are not written to
   the player file. A per-character permanent "bot score" is a thing this
   project is not building. What persists is what an immortal writes down
   deliberately.

5. **Gameplay stays recognisable** (`CLAUDE.md`'s phase-two fidelity
   rule). Anything here that changes what a returning player experiences
   is a deviation and goes in `docs/deviations.md`. Most of this is
   implementation and does not; §4.1's pacing and §6.4's output budget
   are the two that need the entry.

---

## 1. Four adversaries, not one

They get lumped together and they want different things. Separating them
is most of the design.

| | What they are | What they want | What actually hurts |
|---|---|---|---|
| **A. The trigger bot** | A client-side macro/trigger script (zMUD, Mudlet, a 40-line Python socket loop). Predates all of this by thirty years. | Levelling and gold while away from the keyboard. | Nothing much. One is a curiosity. |
| **B. The LLM agent** | A model in a loop with a socket, deciding what to type. | The same, plus "because it is interesting". | Nothing much, at one. |
| **C. The flooder** | Anything sending more than a person can. May be A or B, may be a script with no game knowledge at all. | Disruption, or nothing — often accidental. | The **world goroutine**. See §6. |
| **D. The brigade** | Many accounts, coordinated, usually human-driven. | Harassment, channel spam, market manipulation, mass-report abuse. | Other players, directly. |

**The harm is concentrated in C and D, and it is a function of scale.**
One person botting a character is a rules matter for an immortal. Fifty
agents farming the same zone is an economy problem; two hundred
connections is an availability problem; twenty coordinated accounts
shouting at one player is a harassment problem. A design that spends its
effort identifying *individual* automation, and none on detecting *scale*,
optimises for the case that does not matter.

That is why §5 and §7 share machinery: "is this one player automated" and
"are these twenty players the same person" are the same correlation
question asked twice.

---

## 2. What is already true

The port is not starting from nothing, and several of these are already
better than the C server.

- **`--max-players`**, checked at accept time before anything else
  (`internal/server/listen.go`, `Accept`) — the C's own
  `sockets_connected >= max_players` (`comm.c:1337`).
- **`--max-connections-per-ip`**, default 8, which the C has no
  equivalent of at all. Counted by `perHostKey`, which buckets IPv6 by
  the **/64** rather than the exact address — because privacy extensions
  (RFC 4941) rotate an address on their own and an abuser can pick a
  fresh one per connection for free. That reasoning is already written
  down in `listen.go` and it is load-bearing for everything in §6 and §7.
- **`--login-grace-time`**, a deadline on how long a connection may stay
  unauthenticated (`Server.serve`).
- **`max_bad_pws`** (`internal/game/tuning.go`), wrong passwords per
  connection before disconnect — the C's `config.c:236`. Note what its
  own doc comment says: the count is **per connection**, so reconnecting
  restarts it at zero. That is C fidelity, and it is also not a rate
  limiter.
- **The ban list**, honoured at the name prompt, with `ban`/`unban`
  in-game and four types (`none`/`new`/`select`/`all`).
- **Immortal tooling**: `snoop`, `users`, `dc`, `freeze`, `wizlock`.
- **`--web-captcha`**, an arithmetic challenge in front of `/ws`, whose
  own doc comment is refreshingly honest about scope: it "exists to raise
  the cost of 'point a script at the web port' above 'point a script at
  the telnet port'". That is the right posture and this document extends
  it.
- **A Prometheus registry** (`internal/obs`), so the metrics §9 wants
  have somewhere to go.

**What is missing**, and `docs/operations.md` already says so: a
login-attempt rate limiter. Also missing, and not yet written down
anywhere: any limit at all on *command* rate, on output volume, on
connection *rate* (as opposed to concurrent count), or on new-character
creation rate.

### 2.1 The affordance this port added: GMCP

Worth naming plainly, because it is the single biggest change in
automation cost between the C server and this one, and it was nobody's
mistake.

`internal/telnet` implements GMCP and the server offers it before the
login sequence (`internal/session/protocol.go`). GMCP is a structured,
machine-readable feed of game state — vitals, room, prompt — explicitly
so a client does not have to screen-scrape. `go-port-plan.md` §7 calls it
"the main reason to bother with negotiation at all", and for a modern
human-facing client it is.

It is also exactly what a bot author would otherwise have to write a
parser for. The C server made automation annoying by accident: everything
was prose, the prompt was configurable, and a trigger script broke when
a room description changed. GMCP removes that friction on purpose.

This proposal does **not** suggest removing it. It suggests being
deliberate about it (§4.4), and it suggests that "we offer a clean
machine interface and also want automation to be hard" is a tension to
resolve knowingly rather than a thing to leave implicit.

---

## 3. Why "prevent LLM scripting" is the wrong goal

Stated up front so the rest is read correctly.

**At the protocol level, a well-driven LLM agent and a fast human with a
good client are the same bytes.** There is no attestation in telnet, no
trust anchor in a TCP connection, and no client-side integrity check that
survives a client the player controls. Anything the server can measure —
timing, phrasing, command mix — an adversary who cares can shape. This is
not a gap to be closed by trying harder; it is the shape of the problem.

Worse, **the standard anti-bot toolkit is inverted for this adversary.**
CAPTCHAs, comprehension puzzles, "describe what you see", riddles from an
NPC: these defeat a trigger bot (adversary A) precisely because they
require understanding, and an LLM has understanding. A comprehension
challenge is a *filter that passes LLM agents and blocks macro scripts*,
which is the opposite of what was asked for. Any proposal that reaches
for "make the player prove they understand something" has misidentified
which adversary it is fighting.

So what is actually asymmetric between a human and an LLM agent?

1. **Cost per action.** A human types a command for free. An agent spends
   a model call — money and latency — on every decision. Anything that
   increases *actions required per unit of progress*, or *wall-clock
   required*, is paid for in a currency the human does not have to spend.
2. **Cost per unit of time.** A human idling costs nothing. An agent
   holding a session over eight hours is paying for context, polling, or
   both.
3. **Long-horizon coherence is expensive.** Sustained, consistent,
   many-hour play is where agents get costly and where they get sloppy,
   and it is the exact regime a gold farmer needs.
4. **Setup cost per identity.** One agent is easy. Fifty agents is fifty
   accounts, fifty sessions, and a bill.

Every one of those says the same thing: **attack the economics and the
scale, not the individual's authenticity.** That is what §4 and §7 do.
§5's detection exists to give immortals evidence, not to make the
decision.

There is also a scope question worth settling honestly: a single person
running one LLM agent to play a twenty-five-year-old MUD is, arguably,
not a problem at all. The problem is a hundred of them, or one of them
running the economy. This document is written for the second.

---

## 4. Making automation expensive

### 4.1 Command pacing — the cheapest measure by a distance

**A token bucket per session, refilled at roughly the fastest a person
plays, with a burst allowance large enough that nobody notices it.**

This is the single highest-value item here and it does four jobs at once:

- It is the **only** thing standing between a command flood and the world
  goroutine (§6.5).
- It caps the rate at which *any* automation makes progress, without
  caring whether it is a macro or a model.
- It costs a human literally nothing at a burst of, say, 20 commands and
  a refill of 4–5/second. Nobody sustains five commands a second by hand;
  everybody bursts above it occasionally when a screen of aliases fires.
- It converts "farm this zone in an hour" into "farm this zone in eight
  hours", which for an agent is eight hours of billed context.

Over the bucket, **queue, do not drop and do not disconnect.** A player
who pastes ten lines expects ten lines to run. Dropping them is a bug
report; disconnecting them is a support ticket. A bounded queue that
drains at the refill rate is invisible to a human and is exactly the
brake a bot runs into. Only when the queue itself overflows does the
connection get told to slow down, and only well past that does it get
closed.

Where: `Session.readLoop`'s per-line path (`internal/session/session.go`),
after alias expansion, since `expandAliasedLine` is precisely how one
typed line becomes many commands and the bucket must see the many.

Two gameplay notes, because this touches §0.5. The C server has no such
limit, so this is a deviation and gets an entry. And the numbers have to
be checked against the fastest thing the game legitimately asks for —
combat rounds, `flee` spam in a bad fight, the editor — rather than set
by taste. `test/play` is where that gets pinned.

### 4.2 Rate-limit identity creation, not identity use

Creating a character is currently unbounded: a connection can make one
per grace period, forever, at 8 concurrent per /64.

Proposed: a **new-character creation budget per `perHostKey` bucket**,
over a long window (hours, not seconds), with a low limit (single
digits). This is the measure that most directly raises the cost of
adversary D and of the fifty-agent version of B, and it costs a real new
player nothing — nobody legitimately makes six characters an hour from
one address.

It also has the useful property of being **the only place in this
document where an automatic refusal is unambiguously safe**, because the
thing being refused has no history to lose.

### 4.3 Charge for progress, not for existence

A structural idea rather than a mechanism, and the one most likely to be
rejected on gameplay grounds — recorded because it is the intervention
with the best cost ratio against agents and the worst against humans, and
that trade needs to be made explicitly rather than by default.

Automation converts *wall-clock* into *progress* with no attention cost.
Anything that makes progress depend on something an agent cannot cheaply
buy — diminishing returns on repeated kills of the same mobile, per-hour
caps on gold from a single zone, experience curves that reward breadth
over repetition — attacks exactly that conversion. None of it is
detection; it makes farming not worth automating.

**This is a gameplay change and therefore a `deviations.md` matter and
probably a "no".** It is here because it should be a decision rather than
an omission.

### 4.4 GMCP: deliberate, not free

Given §2.1, three options, in order of preference:

1. **Leave it on and accept it.** Defensible: the port built it for a
   reason, and a determined bot author writes a prose parser anyway. The
   friction saved is real but small.
2. **Gate the expensive packages** — `Char.Vitals` on every prompt is the
   one that turns a socket into an API — behind an opt-in the player sets
   once. Costs a legitimate client one round trip, costs a bot farm
   nothing either, so this is mostly theatre. Named to be dismissed.
3. **Meter it.** Do not restrict GMCP; *count* it. A session driving
   entirely from GMCP with no `look`, no `score`, no scrollback-dependent
   behaviour is a strong signal for §5, and it is free to collect because
   the negotiation state is already tracked (`protocol.supports`).

**Recommendation: 1 plus 3.** Keep the feature, use the fact of it as a
signal.

### 4.5 What not to do, and why

Listed because each of these is the first idea people have.

- **A CAPTCHA at login, or periodically.** Fails §0.3 outright for
  players using a screen reader, fails §3 (an LLM solves arithmetic and
  text puzzles better than the humans it is filtering), and annoys
  exactly the returning player this game exists for. `--web-captcha` is
  defensible only because of what it guards — the *web* port
  specifically — and its own doc comment already scopes it that way.
- **Keystroke-timing biometrics as a gate.** Discriminates against
  anybody using switch access, voice control, an on-screen keyboard, or a
  bad connection. Fine as one advisory signal among many (§5); never as a
  gate.
- **"Are you there?" prompts that disconnect on no answer.** Punishes
  people who step away, which is most people, and an agent answers them
  perfectly.
- **Client fingerprinting as an identity.** TTYPE and NAWS are
  self-reported strings. Useful as a weak correlation signal (§7.2),
  worthless as authentication.
- **Blocking known datacentre ranges.** Blocks VPN users, who are
  disproportionately people with reason to use one. Available to an
  operator as a ban-list entry if they want it; not built in.
- **Shadow-banning suspected bots.** Silently degrading a session you are
  not sure about is how you lose a real player who will never know why,
  and it violates §0.1.

---

## 5. Detection: evidence for a person, not a verdict

The purpose of this section is **to make `snoop` unnecessary as a first
step**, not to replace the immortal at the end of it.

### 5.1 Signals worth computing

All over a rolling in-memory window (§0.4), all per session, all cheap.

| Signal | Why it separates | Weakness |
|---|---|---|
| **Inter-command interval distribution** — variance and floor, not mean | Humans are bursty and irregular; loops are regular. A tight variance is the single most informative number here. | Trivially defeated by adding jitter. Catches the lazy, which is most of them. |
| **Session duration and continuity** | 14 hours without a gap is not a person. | Catches only the blatant. |
| **Command-mix entropy** | Farming is a short cycle repeated. Play is not. | A varied script defeats it. |
| **Reaction to the unpredictable** | See §5.2 — the strongest one. | Costs design effort. |
| **Absence of "human" commands** | Nobody plays for six hours without `look`, `score`, `inventory`, or reading a board. | Easily faked once known. |
| **GMCP-only operation** | §4.4. Free to collect. | Legitimate for a good client. |
| **Perfect recovery** | Never dying, always fleeing at exactly the same hit points, never mistyping. Humans typo. | Good players are consistent too. |

None of these is conclusive and the document should not pretend
otherwise. Their value is **combined and over time**, and their real
value is that they tell an immortal *where to look*.

### 5.2 The one signal that is actually hard to fake

**Unpredictable in-world events that a scripted player handles wrongly.**

Not a challenge, not a gate, not addressed to the player as a test:
ordinary game content whose correct response cannot be precomputed and
whose incorrect response is distinctive. A mobile that says something
that has never been said before. A room whose exits are described
differently this once. An object that behaves unusually when a specific,
never-before-seen thing is done to it.

A trigger bot fails these visibly — it does the thing its script says,
which is now wrong. An LLM agent *handles them*, and that is fine,
because handling them **costs a model call on an unexpected branch**,
which is §3's asymmetry doing its job: the agent pays, the human does
not, and the pattern of who pays is itself observable in the timing.

Two constraints. It must be **content, not a test** — a player who
notices should experience it as the game being interesting, not as being
interrogated. And it must not be **rare enough to be a trap for one
player**; if it fires for everyone, nobody is singled out.

This is design work, not just code, and it is the part of this proposal
most likely to be worth doing and least likely to get done.

### 5.3 Presentation: a `suspect` command, and nothing else

One immortal command. For a named character, or a summary across every
session: the signals above, their values, the window they cover, and no
score, no verdict, no recommendation.

Explicitly **not** a number out of 100. A score invites acting on the
score; the point of this section is to route a human to the evidence.
`snoop` remains what confirms it, and an immortal remains what decides.

---

## 6. Denial of service

### 6.1 The thing that actually breaks

**The world is a single goroutine** (`internal/engine`, `DoSync`;
`CLAUDE.md`'s "Concurrency"). Every command that touches the world
serialises through it.

That is the correct design and it is also the whole DoS surface. A flood
of connections costs goroutines and memory, which is survivable. A flood
of *commands* costs world-goroutine time, and world-goroutine time is the
one resource the entire game shares. One connection sending expensive
commands as fast as it can degrades play for everybody — no packet
volume required, no botnet required, one socket and a loop.

**§4.1's command pacing is therefore not primarily an anti-bot measure.
It is the availability control**, and it should be built for that reason
even if every other word of this document is rejected.

### 6.2 Connection rate, not just connection count

`--max-connections-per-ip` caps *concurrent* connections. It does not cap
the *rate*: connect, be refused at the name prompt, disconnect, repeat —
forever, at whatever rate the network allows, each iteration costing an
accept, a goroutine, a TLS handshake and a greeting write.

Proposed: a per-`perHostKey` **accept-rate** bucket, evaluated before the
TLS handshake, refusing with a plain message and no handshake cost.
Small burst, slow refill. This is also the cheapest defence against
handshake-cost amplification on `--listen-telnets`, where the server does
asymmetric crypto for a peer that has proved nothing.

Below this level — SYN floods, volumetric attacks — is not the game's
problem and the document should say so: that is the kernel's, the
firewall's and the upstream's. Anything this server does about a
saturated link is theatre.

### 6.3 Login attempts

The gap `docs/operations.md` already names. `max_bad_pws` is per
connection and resets on reconnect, so it bounds nothing.

Proposed: a failed-authentication bucket keyed on `perHostKey` **and**
separately on the character name, both with long windows. The
name-keyed one matters because the roster holds twenty-year-old DES
hashes (`--allow-legacy-passwords`), which is a population worth
protecting from a slow distributed guess against one known-valuable
account.

Response is delay, then refusal for a cooling-off period. Not a ban
(§7.4). The message must not distinguish "no such character" from "wrong
password" any more than the current flow already does.

### 6.4 Output amplification

The asymmetry nobody looks for: commands where a few bytes in produce a
lot of bytes out, at world-goroutine cost. `who` on a full server, `look`
in a crowded room, area-wide broadcasts, the help database, long boards.

Proposed: a **per-session output byte budget**, over a window, with the
same queue-don't-drop posture as §4.1 — when a session is over budget its
writes are paced rather than discarded, since a player who asked for a
long help entry should get it, just not instantly.

This is a `deviations.md` entry: the C server paces nothing.

### 6.5 What a graceful degradation looks like

Worth deciding before it is needed, in order of severity, each
reversible:

1. Pace individual sessions (§4.1, §6.4) — invisible.
2. Refuse *new* connections from the noisiest buckets (§6.2) — visible
   only to them.
3. Auto-`wizlock` at level 1: existing players continue, no new logins.
   The command exists; this is automating a decision an immortal already
   has.
4. Refuse all new connections.

Never: disconnecting existing players, and never anything that persists
past the incident.

---

## 7. Brigading

### 7.1 What it actually is here

Concretely, in a MUD, in rough order of likelihood:

- **Channel spam** — `shout`, `gossip`, `holler` from several accounts at
  once, which is a harassment problem and an output-amplification problem
  simultaneously.
- **Mass account creation** to evade a ban or to manufacture presence.
- **Coordinated harassment** of one player: following, killing, blocking
  a room, `tell` spam.
- **Mass-report abuse** — `bug`/`idea`/`typo` in volume. Partially
  already defended: `max_filesize` (`config.c:233`) makes the report
  files refuse to grow past a bound, which is a DoS control the C server
  had by accident.

### 7.2 Correlating sessions is the whole job

One abusive account is a moderation matter. Twenty accounts that are one
person is brigading, and the difference is *correlation*, not per-account
behaviour — each account individually looks fine.

Signals, weakest to strongest, all already available or nearly so:

- Same `perHostKey` bucket. Strong, and evadable by anyone who cares.
- Same client fingerprint (TTYPE, NAWS, GMCP package set, negotiation
  order). Weak individually; distinctive in combination, because most
  people do not vary their client.
- **Temporal correlation** — accounts that log in together, act together
  and leave together. Hard to evade while still being coordinated,
  because coordination is the thing being detected. This is the good one.
- Shared targets: several accounts acting on the same character or the
  same room within a short window.

Note what this shares with §5: it is the same windowed, in-memory,
advisory machinery, keyed on a group instead of a session. Build it once.

### 7.3 Automatic responses that are safe

- **Channel slow mode**, per channel and global, engaged automatically on
  a rate spike and released on its own. Everybody sees the same rule, it
  is reversible, and it is a thing chat systems have done for decades.
- **Per-character channel buckets**, so one account cannot flood a
  channel regardless of what anyone else is doing.
- **New-character creation throttle** (§4.2).
- **Auto-`wizlock`** on a creation or login spike (§6.5.3), with a loud
  log line and an announcement to any immortal online.

Each of these is reversible, applies visibly and equally, and costs a
wrongly-caught player nothing but patience.

### 7.4 Why nothing here bans

The reason is specific, not squeamishness.

**Bans in this server are by site, and IPv6 sites are shared.**
`perHostKey` buckets by /64 for good reasons (§2), and the ban list
matches by site string. An automatic ban on a /64 is an automatic ban on
everyone behind that prefix — which, for a mobile carrier or a CGNAT
deployment, can be a lot of people who did nothing. An automatic ban on
an exact IPv6 address is meaningless, because the abuser rotates it for
free and the legitimate player's address rotates on its own.

So: automatic measures **slow and refuse**; humans **ban**. An immortal
with `ban`, `freeze`, `dc` and the evidence from §5.3 is the escalation
path, and that is a feature.

---

## 8. Configuration

New flags, all with defaults that do something sensible rather than
nothing — a limit nobody turns on is a limit nobody has.

| Flag | Default | What |
|---|---|---|
| `--max-commands-per-second` | ~5 | §4.1 refill |
| `--max-command-burst` | ~20 | §4.1 burst |
| `--max-connections-per-minute` | ~10 | §6.2, per bucket |
| `--max-login-failures` | ~10/hour | §6.3, per bucket and per name |
| `--max-new-characters` | ~5/hour | §4.2, per bucket |
| `--max-output-bytes-per-second` | generous | §6.4 |
| `--abuse-detection` | on | Whether §5's signals are computed at all |

Operator flags rather than `config/game.yaml`, with one exception: if
§4.3 is ever built, its knobs are game tuning and belong in the yaml with
the rest of the gameplay settings. The split follows the existing one —
`internal/config.Config` is how the server is run, `game.GameTuning` is
how the game plays.

Every one of these needs an off switch, because the first thing a
private server behind a VPN wants is none of it.

---

## 9. Metrics and logging

`internal/obs` has the registry; these are counters and histograms on it.

- Connections accepted / refused, by reason.
- Commands executed, queued, and paced, per session and in total.
- Login failures, by bucket.
- Characters created.
- **World-goroutine queue depth and `DoSync` wait time.** This is the
  metric that says whether the game is actually being denied service, and
  it is worth having whether or not anything else here is built.
- Sessions currently above each threshold.

Log lines for anything automatic, at `WARN`, naming the bucket and the
measure taken, plus a `wizlog` to any immortal online for §7.3's global
measures — an automatic action nobody is told about is an automatic
action nobody can undo.

---

## 10. The steps

Ordered by value per unit of work. Rows 1–3 are worth doing regardless of
whether anything after them is.

| # | What | Why here |
|---|---|---|
| 1 | §6.1/§4.1 command pacing, queue-not-drop | The availability fix. Everything else is optional; this is not. |
| 2 | §6.3 login-attempt limiting | Already a named gap in `operations.md`, and the roster is old hashes. |
| 3 | §6.2 connection-rate limiting | Small, closes the reconnect-loop hole. |
| 4 | §9 metrics, especially `DoSync` wait | Cheap, and tells you whether 1–3 worked. |
| 5 | §4.2 creation throttle | The anti-scale measure with the cleanest safety story. |
| 6 | §6.4 output budget | Wants care over what legitimate output looks like. |
| 7 | §7.3 channel slow mode + per-character buckets | Needs the channels; mostly mechanical. |
| 8 | §5.1 signals + §5.3 `suspect` | Only useful once somebody is there to read it. |
| 9 | §7.2 cross-session correlation | Same machinery as 8, keyed differently. |
| 10 | §5.2 unpredictable content | The best idea here and the most work. |

---

## 11. What is not in it

- **Anything that identifies a player as a bot with confidence.** §3.
- **Automated punishment.** §0.1.
- **CAPTCHAs in the game.** §4.5. `--web-captcha` stays as it is, scoped
  to the web port.
- **Removing or crippling GMCP.** §4.4.
- **Volumetric DoS defence.** §6.2 — the network's job.
- **Per-account rate limits that persist across sessions**, which would
  need the retained profile §0.4 declines to keep.
- **A rule about whether botting is *allowed*.** That is a policy for
  whoever runs the server, belongs in `text/policies`, and is upstream of
  every technical measure here. This document assumes such a policy
  exists; it does not write one.

---

## 12. Open questions

1. **Is a solo LLM player actually unwanted?** §3 argues the harm is at
   scale. If one person running an agent is fine, several measures here
   are aimed at nothing and the emphasis shifts entirely to §6 and §7.
   This is the operator's call and it changes the shape of the work.

2. **Is §4.3 (charging for progress) on the table at all?** It is the
   most effective anti-farming idea in this document and the most
   invasive to gameplay. A "no" is a perfectly good answer; an unstated
   "no" is not.

3. **What are the real numbers?** Every threshold in §8 is a guess.
   `test/play` can establish the fastest a legitimate session goes;
   nothing can establish the rest without players.

4. **Does §5.2 belong to this document?** Unpredictable content is a
   content-design idea that happens to have an anti-automation effect.
   It might be better owned somewhere else entirely.

5. **How much of §5 survives §0.4?** In-memory, rolling, session-scoped
   signals are useless for the case where the abuser reconnects. Making
   them useful means retaining something, which is what §0.4 refuses.
   That tension is real and this document resolves it in favour of not
   retaining; somebody may reasonably disagree.

---

## Related documents

- `docs/design/go-port-plan.md` §7 — networking and connection
  hygiene, and what of it landed. §0's fidelity rules bound everything
  here.
- `docs/operations.md` — "Exposure", which is the current answer and
  which this proposal is trying to replace with something better than
  "don't". It is also where the login-attempt limiter §6.3 proposes is
  already recorded as missing.
- `TODO.md` — open item 4, "Hosting and exposure", which is the decision
  this document is downstream of. Nothing here is worth building until
  somebody intends to expose the server; all of it is worth having
  decided before they do.
- `docs/configuration.md` — where §8's flags would be documented.
- `docs/deviations.md` — where §4.1, §4.3 and §6.4 go, being changes the
  C server has no counterpart for.
- `CLAUDE.md` — "Concurrency", for why §6.1 is the section that matters.
