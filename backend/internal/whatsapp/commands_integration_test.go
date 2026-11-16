package whatsapp_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/auth"
	"deuce/backend/internal/club"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
	"deuce/backend/internal/testutil"
	"deuce/backend/internal/whatsapp"
)

const adminPhone = "628111111111@s.whatsapp.net"

type botFixture struct {
	bot      *whatsapp.GroupCommands
	sessions *session.Service
	clubID   uuid.UUID
}

func newBotFixture(t *testing.T) botFixture {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	clubRepo := club.NewRepository(pool)
	playerRepo := player.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)
	sessions := session.NewService(sessionRepo, clubRepo, playerRepo, auth.NewHasher("test"), nil, logger)
	matches := match.NewService(pool, match.NewReadRepository(pool), lock.NoopLocker{}, logger)

	host, err := playerRepo.Create(ctx, "Host", player.Male)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	c, err := clubRepo.CreateClub(ctx, "Bot Club", host.ID, "BOTCLUB1")
	if err != nil {
		t.Fatalf("create club: %v", err)
	}
	return botFixture{
		bot:      whatsapp.NewGroupCommands(c.ID, []string{adminPhone}, sessions, matches, playerRepo),
		sessions: sessions,
		clubID:   c.ID,
	}
}

func jid(t *testing.T, s string) types.JID {
	t.Helper()
	j, err := types.ParseJID(s)
	if err != nil {
		t.Fatalf("parse jid %q: %v", s, err)
	}
	return j
}

func (f botFixture) say(t *testing.T, from whatsapp.Sender, text string) string {
	t.Helper()
	reply, err := f.bot.Handle(context.Background(), from, text)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", text, err)
	}
	return reply
}

func expectContains(t *testing.T, text, reply, want string) {
	t.Helper()
	if !strings.Contains(reply, want) {
		t.Fatalf("%q: reply %q does not contain %q", text, reply, want)
	}
}

// TestIntegration_GroupCommands_FullSession drives a whole session through
// the group commands: setup, assignment, finishing a match (which must free
// the court for the next one), and removing/re-adding a player.
func TestIntegration_GroupCommands_FullSession(t *testing.T) {
	f := newBotFixture(t)
	admin := whatsapp.Sender{JID: jid(t, adminPhone)}
	member := whatsapp.Sender{JID: jid(t, "628999999999@s.whatsapp.net")}

	// Conversation and non-admin mutations.
	if reply := f.say(t, admin, "next week we play at 7?"); reply != "" {
		t.Fatalf("conversation got a reply: %q", reply)
	}
	if reply := f.say(t, admin, "hello everyone"); reply != "" {
		t.Fatalf("conversation got a reply: %q", reply)
	}
	expectContains(t, "start", f.say(t, member, "start"), "admins only")

	expectContains(t, "start", f.say(t, admin, "start"), "created and started")
	expectContains(t, "courts 1", f.say(t, admin, "courts 1"), "1 court(s)")
	for _, name := range []string{"Alice F", "Bob M", "Cara F", "Dan M", "Eve F"} {
		expectContains(t, "add", f.say(t, admin, "add "+name), "added")
	}

	expectContains(t, "next", f.say(t, admin, "next"), "Match assigned")
	expectContains(t, "next (no court)", f.say(t, admin, "next"), "No court is available")

	// Finishing frees the court so another match can be assigned.
	expectContains(t, "done tie", f.say(t, admin, "done 1 21-21"), "tie")
	expectContains(t, "done 2", f.say(t, admin, "done 2 21-15"), "no Court 2")
	expectContains(t, "done 1", f.say(t, admin, "done 1 21-15"), "Court 1 finished 21–15")
	expectContains(t, "done again", f.say(t, admin, "done 1 21-15"), "No match is running")
	expectContains(t, "matches", f.say(t, member, "matches"), "Court 1 — FINISHED (21–15)")
	expectContains(t, "next", f.say(t, admin, "next"), "Match assigned")

	// Remove a waiting player, then bring them back with add.
	sess := f.currentSession(t)
	waitingName := f.someone(t, sess, session.PlayerWaiting)
	expectContains(t, "remove", f.say(t, admin, "remove "+waitingName), "removed")
	if got := f.statusOf(t, sess, waitingName); got != session.PlayerEnded {
		t.Fatalf("after remove: status %s, want ENDED", got)
	}
	expectContains(t, "add back", f.say(t, admin, "add "+waitingName), "added")
	if got := f.statusOf(t, sess, waitingName); got != session.PlayerWaiting {
		t.Fatalf("after re-add: status %s, want WAITING", got)
	}

	playingName := f.someone(t, sess, session.PlayerPlaying)
	expectContains(t, "remove playing", f.say(t, admin, "remove "+playingName), "on court")

	expectContains(t, "cancel", f.say(t, admin, "cancel"), "cancelled")
	expectContains(t, "stop", f.say(t, admin, "stop"), "Session finished")
	expectContains(t, "status", f.say(t, member, "status"), "No session")
}

// TestIntegration_GroupCommands_AdminByAltJID covers groups that address
// members by LID: the admin's phone JID then arrives as the alternate sender.
func TestIntegration_GroupCommands_AdminByAltJID(t *testing.T) {
	f := newBotFixture(t)
	lidAdmin := whatsapp.Sender{JID: jid(t, "123456789@lid"), Alt: jid(t, adminPhone)}
	expectContains(t, "start", f.say(t, lidAdmin, "start"), "created and started")

	lidStranger := whatsapp.Sender{JID: jid(t, "987654321@lid")}
	expectContains(t, "stop", f.say(t, lidStranger, "stop"), "admins only")
}

func (f botFixture) currentSession(t *testing.T) session.Session {
	t.Helper()
	all, err := f.sessions.ListByClub(context.Background(), f.clubID)
	if err != nil || len(all) == 0 {
		t.Fatalf("list sessions: %v (%d)", err, len(all))
	}
	return all[0]
}

// someone returns the display name of a player with the given status, read
// from the public `players` command output ("• Name — STATUS").
func (f botFixture) someone(t *testing.T, sess session.Session, status session.PlayerStatus) string {
	t.Helper()
	reply, err := f.bot.Handle(context.Background(), whatsapp.Sender{}, "players")
	if err != nil {
		t.Fatalf("players: %v", err)
	}
	for _, line := range strings.Split(reply, "\n") {
		name, st, ok := strings.Cut(strings.TrimPrefix(line, "• "), " — ")
		if ok && st == string(status) {
			return name
		}
	}
	t.Fatalf("no player with status %s in:\n%s", status, reply)
	return ""
}

func (f botFixture) statusOf(t *testing.T, sess session.Session, name string) session.PlayerStatus {
	t.Helper()
	reply, err := f.bot.Handle(context.Background(), whatsapp.Sender{}, "players")
	if err != nil {
		t.Fatalf("players: %v", err)
	}
	for _, line := range strings.Split(reply, "\n") {
		n, st, ok := strings.Cut(strings.TrimPrefix(line, "• "), " — ")
		if ok && n == name {
			return session.PlayerStatus(st)
		}
	}
	t.Fatalf("player %q not in:\n%s", name, reply)
	return ""
}
