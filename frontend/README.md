# deuce — frontend

Minimalist SvelteKit PWA for the badminton club backend in `../backend`:
create a club, share a join code, run a live session (queue, courts, match
generation/scoring), and track player ratings — all against the
[REST API](../backend/docs/openapi.yaml) documented there.

Built as a static SPA (`@sveltejs/adapter-static`, `ssr = false`) with a
service worker (`@vite-pwa/sveltekit`) so it can be installed as a PWA.

## Running locally

```bash
npm install
cp .env.example .env   # point VITE_API_BASE_URL at your backend, if not localhost:8080
npm run dev            # http://localhost:5173
```

The backend must be running separately — see `../backend/README.md` (or
`docker compose up` from the repo root).

```bash
npm run build     # production build -> build/
npm run preview   # serve the production build locally
npm run check      # svelte-check (types + a11y)
```

## Auth model

There's no login. Each join/create action returns a bearer token stored in
`localStorage` (`deuce:identity:v1`) — see `src/lib/stores/identity.ts`.
A device can hold multiple identities at once: a **host** token per club it
created, and a **player** token per session it joined. Host-only actions
(start/end session, generate matches, add courts) always use the host
token, even on a device that's also joined its own session as a player —
see `hostToken` vs `token` in the session/match pages.

When a host creates a session, the backend also inserts that host as a
WAITING session player. The host remains a host for management actions and
is also eligible to be assigned to matches without rejoining through the
player flow.

## Assignment modes & auto-fill

Assignment mode is set when the host creates a session, but can be changed at
any time from the session dashboard's host tools (⚡ Host tools →
Matchmaking): a segmented toggle switches between **Automatic** and
**Manual**, and an **Auto-fill** toggle (Automatic mode only) turns the
backend's fully-automatic court filler on or off. Auto-fill is on by default,
so an `AUTOMATIC` session fills its empty courts on its own; turn it off if
you want to tap "Generate match" per court instead. See
`../backend/README.md#assignment-modes--auto-fill`.

## Device identity (v2)

`src/lib/device.ts` generates a random `device_id` once (`crypto.randomUUID()`,
stored in `localStorage`) and `src/lib/api.ts` silently attaches it to every
`createClub`/`joinClub`/`joinSession` call. If the backend has Redis
configured (see `../backend/README.md#device-identity-v2`), a returning
device is recognized within a club and its rating/history carry over
instead of resetting — the response's `you.returning` /`host.returning`
flag is set. This is also how a host regains host access from the same
browser (rejoin via the normal join flow; no separate "recover" step
exists). On a genuinely different device/browser this alone can't recover
host access — see Co-hosts below for the actual fix.

## Co-hosts

A club can have more than one host, via `/club/[clubId]/+page.svelte`'s
Co-hosts & members section (host-only: lists members, "Make co-host" button
per non-host member). The tricky part isn't the promote button — it's that a
*promoted* co-host's own device has no local "I created this club" signal
the way the original creator's browser does, so `isHostOfClub`/`isHostOfSession`
can't just check a local flag.

`identity.ensureCoHostChecked(clubId, token)` (`src/lib/stores/identity.ts`)
is the fix: it calls `GET /clubs/:clubId/me` and caches the result
(`coHostClubs` in the identity store) if the answer is `HOST`. Every page
that needs an accurate `isHost` calls this on load — session page, match
page, club page. One easy-to-reintroduce bug worth knowing about: `$derived`
values that call `identity.*` methods without directly reading `$identity`
somewhere in their own expression *won't* re-run when the store updates
(those methods use the store's `get()` escape hatch internally, which
Svelte's reactivity doesn't track) — every `isHost`/`token`/`hostToken`
derivation in this codebase is deliberately written as
`$derived($identity && identity.xyz(...))` specifically to force that
subscription. Drop the `$identity &&` and a promotion won't visibly take
effect until some *unrelated* reactive value happens to change.

## Live updates

There is no background polling. The session dashboard
(`src/routes/session/[sessionId]/+page.svelte`) fetches once on load, and
again immediately after any action *you* take (start/end session, add a
court, generate/confirm a match, change your own status) — those already
refresh without waiting on anything. To see a change made from a *different*
device (another player joining, the host acting from elsewhere), pull down
from the top of the session page, or reload the page. This was a deliberate
choice, not a missing feature: eventual consistency is fine for a casual
club session, and it avoids the backend's per-IP rate limit (see
`../backend/README.md`) being tripped by every open tab polling on a timer.
The only thing that still ticks on its own is the waiting-time clock next to
each queued player — pure client-side math against the last-fetched
timestamp, no network involved.

## Known gaps (backend limitation, not fixable from the frontend alone)

The API never returns which players are in a match (`MatchDTO` has no
player fields). The session dashboard infers court occupants client-side
(diffing who flipped to `PLAYING`, or caching what was submitted for a
manual match confirm, in `src/lib/stores/matchTeams.ts`) — this is
best-effort and lost on a hard refresh for automatically-generated matches.
See `backend/docs/v2-plan.md` §9 for the proposed API fix.
