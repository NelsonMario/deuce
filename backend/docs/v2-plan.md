# V2 Plan

V1 intentionally keeps matchmaking, rating, and identity simple so real club
data can be collected before investing in anything more sophisticated. This
document captures what V1 deferred and why, so V2 work has context instead
of re-litigating the same tradeoffs.

## 1. Persistent player identity — IMPLEMENTED (device-bound, see `internal/device`)

**V1**: every join (club or session) creates a brand-new `Player` row. There
is no way to recognize "the same human" across two joins — a player who
loses their token starts over at rating 1000.

**V2 (shipped)**: an optional lightweight identity layer, not a full
account system:
- The client generates a random `device_id` once (`localStorage`) and sends
  it as an optional field on `CreateClub`/`JoinClub`/`JoinSession`.
- The backend keeps a tiny Redis index (`device:<id>:club:<clubID> ->
  playerID`, ~180-day rolling TTL) mapping a device to the player it was
  previously recognized as, **scoped per club**. Postgres stays the source
  of truth for the player/rating/history rows themselves.
- A recognized rejoin reuses the existing player (rating, history, and club
  role — e.g. `HOST` — all carry over) instead of creating a new one, and
  issues a fresh bearer token. This is also how a host regains host access
  from the same device: rejoining via `/clubs/:clubId/join` is recognized
  and returns their same `HOST` player.
- Redis is optional and best-effort: unset/unreachable degrades silently to
  plain V1 behavior rather than failing requests.
- **What this deliberately does not do**: no cross-device or cross-browser
  recovery, no login, no password. That still requires full accounts
  (email/OAuth) and remains out of scope — only build it if clubs actually
  ask for cross-device profile sync, per the original V1 deferral (spec
  section 33).

## 2. Matchmaking: wider candidate search

**V1**: `GenerateMatch` fixes the candidate group to exactly the
top-priority players (top 4, or top 2 males + top 2 females for mixed) and
only optimizes the 2v2 split within that fixed group. This is deterministic,
fast, and matches the spec's worked example exactly, but it means a
slightly-better-balanced group one priority-rank down is never considered.

**V2**: widen the search to a configurable window (e.g. top 8–12 eligible
players by priority), brute-force `C(n,4)` candidate groups, and score each
group on a weighted combination of (a) rotation priority satisfied and (b)
rating balance, instead of treating priority as a hard pre-filter. Club size
in V1's target market keeps this brute-forceable (see spec section 15).

## 3. Fairness score

**V1**: collects raw counters only — `matches_played`, `wins`, `losses`,
accumulated waiting seconds. No composite fairness metric (spec section 22
explicitly says "do NOT implement a complicated fairness score yet").

**V2**: once a few months of real session data exist, define a fairness
score (e.g. variance of waiting time across a session, or matches-played
Gini coefficient) and surface it to hosts as a session-quality signal, or
even feed it back into matchmaking priority as a fourth tiebreaker.

## 4. Rating model refinements

**V1**: single global Elo-style rating (K=32), gender-independent, no decay,
no format-specific ratings (a player has one rating across mixed/men's/
women's doubles).

**V2** candidates, evaluate independently — each adds real complexity and
should only be built if V1 data shows a need:
- Separate ratings per format (mixed vs men's vs women's), since skill
  transfer across formats isn't 1:1.
- Rating decay for inactive players so a player who hasn't played in 6
  months doesn't retain a stale high rating.
- Confidence/uncertainty tracking (Glicko-2 style) so a brand-new player's
  first few matches move their rating faster than an established player's.
- Partner-chemistry adjustments — explicitly out of scope per spec section
  33; only revisit if hosts ask for it.

## 5. Session-player state history

**V1**: stores only `waiting_started_at` + `accumulated_waiting_seconds` on
`session_players` — enough to compute total waiting time, but not a full
timeline of state transitions (spec section 20 explicitly allows this
simplification: "If necessary, introduce a session-player state history
table later").

**V2**: add a `session_player_state_history` table
(`session_player_id, from_status, to_status, occurred_at`) to support:
- Per-state-visit duration analytics (e.g. "average BREAK length").
- Audit trail for disputes ("why was I marked ENDED?").
- Replaying a session's timeline for a future "session summary" screen.

## 6. Manual assignment UX

**V1**: `manual/recommend` and `manual/confirm` are two separate calls; the
host is trusted to pass a consistent `format`/player set between them (no
server-side "recommendation token" tying the two together).

**V2**: consider a short-lived recommendation token so `confirm` can
validate it's confirming (an override of) the exact recommendation it
returned, useful once there's a frontend that shows the recommendation and
lets the host drag-and-drop players between teams before confirming.

## 7. Multi-court concurrency at scale

**V1**: match generation locks every `WAITING` session_player row for the
*entire session* before selecting a group of 4 (spec section 16). This
guarantees correctness but serializes concurrent generation requests within
one session — acceptable at typical club sizes (a handful of courts, a few
dozen players) but would become a bottleneck at large-scale multi-court
tournaments.

**V2**: if a club needs many simultaneous courts, revisit to lock only a
bounded candidate window (e.g. advisory locks per priority bucket) instead
of the full waiting pool, trading a little candidate-search completeness for
better parallelism.

## 8. Observability

**V1**: structured `slog` logs for lifecycle events (`club_created`,
`match_generated`, `rating_updated`, etc.) plus request-level logging
middleware. No metrics/tracing.

**V2**: add Prometheus metrics (match generation latency, matchmaking
candidate-pool size, rating-change distribution) and OpenTelemetry tracing
across the generate/start/finish transaction boundaries once there's
production traffic worth alerting on.

## 9. Frontend-facing needs (not backend work, but backend should anticipate)

A minimal SvelteKit PWA now exists (`../frontend`) covering the full flow
(create/join, live session dashboard, match generation/scoring, player
profiles). It deliberately does **not** poll — it fetches on load and after
each of the viewer's own actions, and otherwise relies on a manual "Refresh"
gesture/icon, accepting eventual consistency for changes made by other devices
(see its README's "Live updates" section for why: an earlier polling
implementation had a reactivity bug that caused a request feedback loop,
and even a correct polling implementation would still contend with the
per-IP rate limit described in `../README.md#rate-limiting` once more than
one tab/device is open). It also
doesn't yet expose match rosters beyond what it infers client-side (see its
README/notes) since `MatchDTO` has no player fields — a real gap worth
closing before V2 relies on the frontend to display "who's on which court"
reliably.

- If real-time push (WebSocket/SSE) is ever added, it would let the
  frontend drop the manual refresh gesture/icon in favor of true live updates —
  but that's a meaningful backend investment, not yet justified for a
  casual club session where "click refresh" is an acceptable interaction.
  V1/early-V2's aggregated session-detail endpoint is designed to be easy to
  diff client-side whenever this happens.
- `MatchDTO` (and `ListSessionMatches`) still don't return which four
  players are in a match. The frontend currently infers this client-side
  (diffing who flipped to `PLAYING`, or caching what it submitted for manual
  confirms) — fragile and lost on refresh. V2 should add match rosters to
  the API directly (e.g. a `players: [{player_id, team}]` field) once the
  Redis/device work above has settled.
- QR code generation for club/session join codes is a frontend/asset
  concern — the backend only needs to keep issuing/validating the
  underlying join code string, which it already does.
