package whatsapp_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/auth"
	"deuce/backend/internal/club"
	"deuce/backend/internal/device"
	"deuce/backend/internal/lock"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
	"deuce/backend/internal/testutil"
	"deuce/backend/internal/whatsapp"
)

const (
	adminGroup    = "120363000000000001@g.us"
	announceGroup = "120363000000000002@g.us"
	otherGroup    = "120363000000000003@g.us"
)

type fakeAnnouncer struct {
	mu   sync.Mutex
	msgs []string
}

func (a *fakeAnnouncer) Announce(_ context.Context, text string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.msgs = append(a.msgs, text)
	return nil
}

func (a *fakeAnnouncer) last() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.msgs) == 0 {
		return ""
	}
	return a.msgs[len(a.msgs)-1]
}

type botFixture struct {
	bot       *whatsapp.GroupCommands
	announcer *fakeAnnouncer
	players   player.Repository
	clubs     *club.Service
}

func newBotFixture(t *testing.T) botFixture {
	pool := testutil.NewTestPool(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	clubRepo := club.NewRepository(pool)
	playerRepo := player.NewRepository(pool)
	sessionRepo := session.NewRepository(pool)
	hasher := auth.NewHasher("test")
	clubs := club.NewService(clubRepo, playerRepo, hasher, device.NoopLinker{}, 8, logger)
	sessions := session.NewService(sessionRepo, clubRepo, playerRepo, hasher, nil, logger)
	reads := match.NewReadRepository(pool)
	matches := match.NewService(pool, reads, lock.NoopLocker{}, logger)
	config := whatsapp.NewConfigStore(pool)
	matches.SetAnnouncementOutbox(whatsapp.NewRepository(pool, config))
	ann := &fakeAnnouncer{}
	return botFixture{
		bot:       whatsapp.NewGroupCommands(config, clubs, sessions, matches, reads, playerRepo, ann),
		announcer: ann,
		players:   playerRepo,
		clubs:     clubs,
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

func (f botFixture) send(t *testing.T, chat string, fromMe bool, text string) string {
	t.Helper()
	reply, err := f.bot.Handle(context.Background(), whatsapp.Request{Chat: jid(t, chat), FromMe: fromMe, Text: text})
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", text, err)
	}
	return reply
}

// admin sends a command in the admin group from an admin's own phone.
func (f botFixture) admin(t *testing.T, text string) string {
	t.Helper()
	return f.send(t, adminGroup, false, text)
}

func expect(t *testing.T, label, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%s: reply %q does not contain %q", label, got, want)
	}
}

func (f botFixture) setUp(t *testing.T) {
	t.Helper()
	expect(t, "setup admin", f.send(t, adminGroup, true, "setup admin Tiger Club"), "admin group")
	expect(t, "setup announce", f.send(t, announceGroup, true, "setup announce"), "announcement group")
}

func TestIntegration_Setup(t *testing.T) {
	f := newBotFixture(t)

	// Before setup, commands explain how to set up.
	expect(t, "before setup", f.admin(t, "status"), "not set up")
	// setup is only accepted from the bot's own phone.
	if reply := f.send(t, adminGroup, false, "setup admin Tiger Club"); reply != "" {
		t.Fatalf("setup from a non-bot phone outside the admin group should be ignored, got %q", reply)
	}
	expect(t, "usage", f.send(t, adminGroup, true, "setup admin"), "Usage")
	expect(t, "setup admin", f.send(t, adminGroup, true, "setup admin Tiger Club"), "*Tiger Club* (new)")
	expect(t, "non-bot setup in admin group", f.send(t, adminGroup, false, "setup announce"), "bot's own phone")
	expect(t, "announce in admin group", f.send(t, adminGroup, true, "setup announce"), "This is the admin group")
	expect(t, "setup announce", f.send(t, announceGroup, true, "setup announce"), "announcement group")
	expect(t, "admin in announce group", f.send(t, announceGroup, true, "setup admin"), "This is the announcement group")

	status := f.send(t, adminGroup, true, "setup status")
	for _, want := range []string{"Tiger Club", "Admin group: set", "Announcement group: set", "This group: admin group"} {
		expect(t, "setup status", status, want)
	}
	expect(t, "other group status", f.send(t, otherGroup, true, "setup status"), "not used by the bot")

	// Re-running setup admin keeps the club.
	expect(t, "re-setup", f.send(t, otherGroup, true, "setup admin Something Else"), "Club: *Tiger Club*")
}

func TestIntegration_RosterAndGrades(t *testing.T) {
	f := newBotFixture(t)
	f.setUp(t)

	reply := f.admin(t, "member\nAlice F A\nBob M B\nMary Jane F C\nDan M E")
	expect(t, "member bulk", reply, "4 added, 0 updated")

	members := f.admin(t, "member")
	for _, want := range []string{"Alice (F) — A · 1400", "Bob (M) — B · 1200", "Mary Jane (F) — C · 1000", "Dan (M) — E · 600"} {
		expect(t, "members", members, want)
	}

	// Re-import keeps earned ratings but updates the grade.
	expect(t, "re-import", f.admin(t, "member Bob M A"), "0 added, 1 updated")
	expect(t, "member info", f.admin(t, "member bob"), "Bob (M) — A · 1200")
	expect(t, "member unknown", f.admin(t, "member Nobody"), "not a member")
	expect(t, "grade show", f.admin(t, "grade Mary Jane"), "Mary Jane (F) — C · 1000")

	// Old names still work.
	expect(t, "legacy roster", f.admin(t, "roster\nEve F D"), "1 added")
	expect(t, "legacy members", f.admin(t, "members"), "Eve (F) — D · 800")

	// grade resets the rating explicitly.
	expect(t, "grade", f.admin(t, "grade bob A"), "rating reset to 1400")
	expect(t, "grade unknown", f.admin(t, "grade Nobody B"), "not a member")
	expect(t, "grade bad", f.admin(t, "grade Bob Z"), "A, B, C, D or E")
}

func TestIntegration_FullSession(t *testing.T) {
	f := newBotFixture(t)
	f.setUp(t)
	f.admin(t, "member\nAlice F A\nBob M B\nCara F B\nDan M C\nEve F D\nFrank M C")

	expect(t, "start", f.admin(t, "start"), "created and started")
	expect(t, "start announce", f.announcer.last(), "Session started")
	expect(t, "courts", f.admin(t, "courts 2"), "2 court(s)")
	expect(t, "courts list", f.admin(t, "courts"), "Court 2 — free")
	expect(t, "manual", f.admin(t, "manual"), "Manual")

	reply := f.admin(t, "in Alice Bob Cara Dan Zed")
	expect(t, "in", reply, "Checked in (4)")
	expect(t, "in unknown", reply, "Not members: Zed")
	reply = f.admin(t, "in Zed M D, Yan F C")
	expect(t, "in new", reply, "Checked in (2): Zed, Yan")
	expect(t, "in new members", reply, "New members: Zed (D), Yan (C)")
	expect(t, "new member rating", f.admin(t, "member Zed"), "Zed (M) — D · 800")
	expect(t, "out", f.admin(t, "out Yan"), "out of the rotation")
	expect(t, "legacy add", f.admin(t, "add Yan"), "Checked in (1): Yan")
	expect(t, "legacy remove", f.admin(t, "remove Yan"), "out of the rotation")

	// Manual assignment.
	expect(t, "suggest", f.admin(t, "suggest Alice Bob Cara Dan"), "Most balanced")
	expect(t, "assign bad court", f.admin(t, "assign 9 Alice Bob vs Cara Dan"), "no Court 9")
	expect(t, "assign unknown", f.admin(t, "assign 1 Alice Bob vs Cara Nobody"), "Not members: Nobody")
	expect(t, "assign", f.admin(t, "assign 1 Alice Bob vs Cara Dan"), "Court 1: Alice / Bob vs Cara / Dan")
	expect(t, "assign busy player", f.admin(t, "assign 2 Alice Zed vs Cara Dan"), "Could not assign")

	// Finishing posts the result to the announcement group.
	expect(t, "done", f.admin(t, "done 1 15-21"), "Court 1 finished 15–21")
	expect(t, "result announce", f.announcer.last(), "Cara / Dan beat Alice / Bob 21–15")
	expect(t, "in on court", f.admin(t, "assign 1 Alice Bob vs Cara Dan"), "Court 1")
	expect(t, "already on court", f.admin(t, "in Alice"), "Already on court: Alice")
	expect(t, "cancel", f.admin(t, "cancel"), "cancelled")
	expect(t, "in for set", f.admin(t, "assign 1 Alice Bob vs Cara Dan"), "Court 1")
	expect(t, "set bench", f.admin(t, "set Alice Eve"), "now playing instead of Alice")
	expect(t, "cancel court", f.admin(t, "cancel 1"), "Court 1 match cancelled")
	expect(t, "next", f.admin(t, "next"), "assigned")

	status := f.admin(t, "status")
	t.Logf("status:\n%s", status)
	for _, want := range []string{"active · manual", "Court 1: ", " vs ", "Court 2: free", "*Waiting (", "*Finished (1)*", "Cara / Dan beat Alice / Bob 21–15"} {
		expect(t, "status", status, want)
	}
	expect(t, "legacy matches", f.admin(t, "matches"), "*Finished (1)*")
	expect(t, "auto", f.admin(t, "auto"), "Auto")
	expect(t, "status auto", f.admin(t, "status"), "active · auto")
	expect(t, "manual again", f.admin(t, "mode manual"), "Manual")

	// Stop: recap to admins, short notice to players.
	recap := f.admin(t, "stop")
	t.Logf("recap:\n%s", recap)
	for _, want := range []string{"Session finished", "Session recap", "1 match", "Cara", "Alice"} {
		expect(t, "recap", recap, want)
	}
	expect(t, "stop announce", f.announcer.last(), "Session finished")
}
