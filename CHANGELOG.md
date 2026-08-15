# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/) —
`MAJOR.MINOR.PATCH`:

- **MAJOR** — breaking API or data changes.
- **MINOR** — new features, backwards-compatible.
- **PATCH** — bug fixes and other backwards-compatible changes.

The current version is shown in the backend's `/healthz` response and in
the frontend footer, so a bug report can always be tied to a specific
build.

## [Unreleased]

### Added

- Mid-session matchmaking controls: a host can switch a session between
  Automatic and Manual assignment, and toggle fully-automatic court
  auto-fill, at any point — not just when creating the session.
  (`PATCH /sessions/:sessionId/assignment-mode` and
  `PATCH /sessions/:sessionId/auto-fill` on the backend, plus the
  matchmaking controls in the session dashboard's host tools.)

## [0.1.0] - 2026-08-11

Baseline release — the app as it stands today, versioned for the first time.

### Added

- Club creation with join codes, and support for multiple co-hosts per club.
- Live sessions with courts and player queueing (waiting / playing / break).
- Automatic and manual doubles matchmaking, balanced by player rating and
  wait time, including mixed doubles.
- Elo-style player ratings, updated after every finished match.
- Optional device-identity recognition (Redis-backed) so a returning
  player or host keeps their rating, history, and role across joins,
  without accounts or passwords.
- SvelteKit PWA frontend, installable on mobile, talking to the Go/Fiber
  backend over a documented REST API (`backend/docs/openapi.yaml`).
