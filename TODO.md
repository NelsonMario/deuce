# TODO — next minor updates

Not scoped/estimated yet — just captured so nothing gets lost. Pull items
into an actual plan before starting one.

## Process (do first)

- [X] **Add versioning / release process.** Semantic versioning (`CHANGELOG.md`,
  baselined at `0.1.0`), backend version surfaced on `GET /healthz`
  (`backend/internal/version`, set via `-ldflags` at build time), and a
  version string in the frontend footer (`frontend/vite.config.ts` bakes
  in `package.json`'s version as `__APP_VERSION__`). Still open: actually
  cutting a `v0.1.0` git tag, and wiring the backend Docker build's
  `VERSION` build-arg to `git describe` in CI instead of defaulting to
  `dev`.

## Session behavior

- [X] **Let assignment mode change after the session starts, not just at creation.**
  Right now `assignment_mode` (Automatic/Manual) is fixed when the
  session is created and can't be changed once it's `ACTIVE`. The host
  should be able to toggle it mid-session (per match), not have it
  locked in before the first game is even played.
- [X] **Fully-automatic mode: auto-fill every empty court, no host trigger.**
  Today "Automatic" still requires the host to click "Generate match"
  per court. Consider a mode where the backend (or a frontend loop)
  automatically generates a match for *any* court that goes `AVAILABLE`
  — the host stops being a required manual trigger and can just let it
  run. Needs thought on where this lives (backend background job vs.
  frontend auto-clicking) and how a host pauses it if they want to step
  back in.
- [X] **Relax "Mixed" doubles to any gender split, not just 2M+2F.**
  `matchmaking.GenerateMatch`/`bestMixedSplit` (`internal/matchmaking/domain.go`)
  currently hard-requires exactly 2 males + 2 females for
  `MIXED_DOUBLES`, and errors out otherwise. Allow uneven splits too
  (e.g. 3M vs 1F, 1M vs 3F) instead of rejecting the match — "mixed"
  should mean "gender doesn't gate it," not "must be balanced 2-and-2."
  Needs a decision on how team assignment/rating balance works for
  uneven splits, not just a validation relaxation.

## Player-facing

- [X] **Make a player's own rating easy to check, not just a one-off toast.**
  Right now rating only surfaces in the "welcome back" toast on rejoin
  (or by navigating to `/player/:id`). Show it persistently in the
  "You" card on the session dashboard.
- [X] **Show ratings on the admin/host player lists (Playing / Waiting /
  Break), not just in the Manual-mode picker.** Ratings are currently
  only fetched/shown inside the manual match-builder's pick-chips. Add
  them to the regular player-list rows so the host can see them at a
  glance in any assignment mode.
- [ ] **Add a per-club leaderboard.** Backed by plain Postgres (join
  `club_members` + `player_ratings`, `ORDER BY rating DESC`) — not
  Redis; at current scale (25 players/week) a sorted-set cache would be
  solving a performance problem that doesn't exist. Scoped per club
  (ranks people who actually play together, not every player across
  every unrelated club). Each row: rank, name, rating, lifetime matches
  played. Still open: where it lives in the UI — a section on the club
  page, inside the session dashboard, or both — decide when actually
  scoping this.

## UX / Interaction

- [X] **Reduce information density; prefer one-button actions over forms/lists
  where possible.** Referenced idea: a collapsible/floating sidebar in
  the style of Sword Art Online's HUD — minimal, out-of-the-way, one
  tap to act. Worth a real design pass rather than incremental tweaks
  to the current card-heavy layout.
- [X] **Consider drag-and-drop for the manual match builder.** Dragging
  players onto Team A/Team B (or onto a court) instead of
  checkbox-select-then-confirm — needs a touch-friendly implementation
  given majority-phone usage (see the phone-responsiveness item above).

## Branding

- [X] **Rename the app from "tatakae" to "deuce."** Touches: Go module name
  (`tatakae/backend` in `go.mod` and every import path — a real
  find/replace across the codebase, not just a display string),
  frontend `package.json`/title/favicon/PWA manifest, docs, and the
  Redis key prefix if it's ever made human-readable. Do as one focused
  pass, not piecemeal, since the Go module rename touches every file.

## Backend / ops

- [X] **Re-enable the rate limiter** (`internal/httpapi/middleware/middleware.go`,
  registration commented out in `router.go`). Do this alongside — not
  instead of — keying it by bearer token rather than IP, since IP-keying
  is what caused it to trip during shared-network testing. See
  `backend/README.md#rate-limiting`.

## Redis ideas (beyond the device-identity linking already built)

- [X] **Shared, token-keyed rate limiter.** Replace Fiber's in-memory,
  IP-keyed limiter with a Redis-backed one keyed by bearer token —
  fixes both the "shared WiFi shares one budget" problem and the
  "per-process only, doesn't share across replicas" limitation. Pairs
  with the "re-enable the rate limiter" item above; do them together.
- [ ] **"My clubs" reverse index** (`device:<id>:clubs` → SET of club_ids).
  Lets the frontend show "your clubs" on load without a new Postgres
  query. Solves the gap where there's currently no way to look up what
  clubs a device has hosted/joined.
- [X] **Double-tap guard on match generation.** Short-lived lock (`SETNX`
  + a few seconds TTL) keyed by session_id/court_id before calling
  `GenerateMatch`, so a double-click or slow-network retry can't create
  two matches for the same court.
- [X] **Coordination lock for the "fully-automatic auto-fill" idea** (see
  Session behavior above), if that becomes a background loop instead
  of a host button-click — need exactly one process acting per session
  at a time.

se

**Capacity check (done 2026-08-11, at 100 people/week for a year, worst-case/pessimistic assumptions):**

| Feature                                            | Driver                                      | Footprint                                     |
| -------------------------------------------------- | ------------------------------------------- | --------------------------------------------- |
| Device-identity core (already built)               | 5,200 unique devices/yr × ~200 B           | ~1.04 MB                                      |
| Shared rate limiter (heavy sliding-window variant) | ~5,000 concurrent active users, same minute | ~4 MB                                         |
| My-clubs reverse index                             | same 5,200 devices × ~200 B                | ~1.04 MB                                      |
| Double-tap lock                                    | transient, ~50 concurrent courts peak       | ~20 KB                                        |
| Auto-fill coordination lock                        | transient, ~50 concurrent sessions peak     | ~10 KB                                        |
| **Total**                                    |                                             | **~6.55 MB — 22% of the 30 MB budget** |

That's a pessimistic ceiling (zero repeat players ever + an unrealistic
multi-club concurrent spike for the limiter); realistic single-club usage
with actual repeat attendance would likely be under 5%. Conclusion: all four
(rate limiter, reverse index, both locks) are comfortably maintainable
together even at 4x current volume — capacity is not a blocker for any of
them, only implementation priority is.

## Design

- [X] **Move the UI away from feeling generic/"AI-generated."** Explore a
  pop-art direction (bold flat colors, high contrast, maybe halftone/
  comic accents) while keeping the overall layout minimalist — this is
  about visual identity, not adding chrome/clutter.
- [X] **Verify responsiveness across the full range of phone widths, portrait first.**
  This app will likely be used mostly on phones, held vertically —
  design/test for that as the primary case, not desktop-with-a-mobile-
  afterthought. Check small widths too (e.g. 320-360px, older/smaller
  phones), not just the ~390px we've spot-checked so far.

## Open product questions

- [ ] **How should "anonymous"/pre-listed players work?** In practice, hosts
  often coordinate attendance beforehand outside the app (e.g. a
  numbered WhatsApp list: "1. Alice, 2. Bob, ..."), before anyone has
  opened the app or gotten a token. Right now a "player" only exists
  once someone actually joins through the app — there's no way to
  pre-seed a roster and have people "claim" their slot later. Worth
  thinking through: does the host bulk-add placeholder names that get
  claimed on join, or does the app just stay out of pre-session
  coordination entirely and only take over once people start joining
  for real? No decision yet — needs product thinking before any
  backend/frontend design.
