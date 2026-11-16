package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/club"
	"deuce/backend/internal/database/db"
	"deuce/backend/internal/gemini"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

// Sender identifies who wrote a group message. WhatsApp groups may address
// members by LID (`123@lid`) instead of phone number; Alt then carries the
// phone-number JID (or vice versa).
type Sender struct {
	JID types.JID
	Alt types.JID
}

// Request is one command message the dispatcher has authorized.
type Request struct {
	// Chat is the group the command was typed in; replies go back there.
	Chat types.JID
	// FromMe is true when the message was typed on the bot's own phone,
	// which is what authorizes `setup`.
	FromMe bool
	Text   string
}

// Announcer posts to the announcement group.
type Announcer interface {
	Announce(ctx context.Context, text string) error
}

// GroupCommands runs bot commands. The dispatcher only passes on commands
// from the admin group (or `setup`), so every command here is authorized.
type GroupCommands struct {
	config    ConfigStore
	clubs     *club.Service
	sessions  *session.Service
	matches   *match.Service
	reads     match.ReadRepository
	players   player.Repository
	announcer Announcer
	gemini    *gemini.Client
}

func NewGroupCommands(config ConfigStore, clubs *club.Service, sessions *session.Service, matches *match.Service, reads match.ReadRepository, players player.Repository, announcer Announcer) *GroupCommands {
	return &GroupCommands{config: config, clubs: clubs, sessions: sessions, matches: matches, reads: reads, players: players, announcer: announcer}
}

// SetGemini configures the optional Gemini AI client for natural language commands.
func (c *GroupCommands) SetGemini(client *gemini.Client) {
	c.gemini = client
}

// Handle runs one command and returns the reply for the chat it came from.
// Messages that are not commands return an empty reply.
func (c *GroupCommands) Handle(ctx context.Context, req Request) (string, error) {
	cmd, ok := ParseCommand(req.Text)
	if !ok {
		return "", nil
	}
	cfg, err := c.config.Get(ctx)
	if err != nil {
		return "", err
	}
	if cmd.Name == "setup" {
		return c.setup(ctx, req, cfg, cmd)
	}
	if cmd.Name == "help" {
		return helpText(), nil
	}
	if cfg.ClubID == nil {
		return "ℹ️ The bot is not set up yet. On the bot's phone, type `setup admin <club name>` in the admin group.", nil
	}
	clubID := *cfg.ClubID

	// Club-level commands work without a session.
	switch cmd.Name {
	case "member":
		return c.member(ctx, clubID, cmd.Rest)
	case "grade":
		return c.grade(ctx, clubID, cmd.Args)
	}

	sess, err := c.currentSession(ctx, clubID)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return "⚠️ Could not load the current session.", err
	}
	if errors.Is(err, session.ErrNotFound) {
		if cmd.Name == "chat" {
			return c.chat(ctx, req, clubID, nil, cmd.Rest)
		}
		if cmd.Name != "start" {
			return "ℹ️ No session is running. Use `start` to create one.", nil
		}
		created, err := c.sessions.CreateSession(ctx, session.CreateSessionInput{
			ClubID: clubID, Name: "WhatsApp Session " + time.Now().Format("2 Jan 2006"), AssignmentMode: session.AssignmentAutomatic,
		})
		if err != nil {
			return "", err
		}
		if _, err := c.sessions.StartSession(ctx, created.ID); err != nil {
			return "", err
		}
		c.announce(ctx, req, "🏸 *Session started!* Lineups will be posted here.")
		return "🏸 Session created and started. Next: `courts <n>`, then `in <names>`.", nil
	}

	switch cmd.Name {
	case "chat":
		return c.chat(ctx, req, clubID, &sess, cmd.Rest)
	case "status":
		return c.status(ctx, sess)
	case "start":
		if sess.Status == session.StatusActive {
			return "ℹ️ Session is already active.", nil
		}
		if _, err := c.sessions.StartSession(ctx, sess.ID); err != nil {
			return "", err
		}
		c.announce(ctx, req, "🏸 *Session started!* Lineups will be posted here.")
		return "🏸 Session started.", nil
	case "stop":
		if sess.Status != session.StatusActive {
			return "ℹ️ Session has not started yet.", nil
		}
		if _, err := c.sessions.EndSession(ctx, sess.ID); err != nil {
			return "", err
		}
		c.announce(ctx, req, "🏁 *Session finished.* Thanks for playing!")
		recap, err := c.recap(ctx, clubID, sess)
		if err != nil {
			return "🏁 Session finished. (The recap could not be built.)", err
		}
		return "🏁 Session finished.\n\n" + recap, nil
	case "auto":
		return c.auto(ctx, sess)
	case "manual":
		return c.manual(ctx, sess)
	case "next":
		return c.next(ctx, sess)
	case "in":
		return c.checkIn(ctx, clubID, sess, cmd)
	case "out":
		return c.out(ctx, sess, cmd.Rest)
	case "courts":
		return c.courts(ctx, sess, cmd.Args)
	case "suggest":
		return c.suggest(ctx, clubID, sess, cmd.Rest)
	case "assign":
		return c.assign(ctx, clubID, sess, cmd.Rest)
	case "done":
		return c.done(ctx, req, sess, cmd.Args)
	case "cancel":
		return c.cancel(ctx, sess, cmd.Args)
	case "set":
		return c.setPlayers(ctx, sess, cmd.Rest)
	}
	return "", nil
}

// ---------------------------------------------------------------- setup

func (c *GroupCommands) setup(ctx context.Context, req Request, cfg Config, cmd Command) (string, error) {
	chat := req.Chat.ToNonAD().String()
	if !req.FromMe {
		if chat == cfg.AdminGroup {
			return "⛔ `setup` must be typed on the bot's own phone.", nil
		}
		return "", nil
	}
	if req.Chat.Server != types.GroupServer {
		return "⚠️ Type `setup` inside a group.", nil
	}
	sub := ""
	if len(cmd.Args) > 0 {
		sub = strings.ToLower(cmd.Args[0])
	}
	switch sub {
	case "admin":
		if chat == cfg.AnnounceGroup {
			return "⚠️ This is the announcement group. Create a separate group for admins and type `setup admin` there.", nil
		}
		clubName := strings.TrimSpace(strings.Join(cmd.Args[1:], " "))
		var clubID uuid.UUID
		var clubLine string
		switch {
		case cfg.ClubID != nil:
			clubID = *cfg.ClubID
			cl, err := c.clubs.GetClub(ctx, clubID)
			if err != nil {
				return "", err
			}
			clubLine = fmt.Sprintf("Club: *%s*", cl.Name)
		case clubName == "":
			return "Usage: `setup admin <club name>` (the club is created the first time).", nil
		default:
			cl, created, err := c.clubs.FindOrCreateByName(ctx, clubName)
			if err != nil {
				return "", err
			}
			clubID = cl.ID
			clubLine = fmt.Sprintf("Club: *%s*", cl.Name)
			if created {
				clubLine += " (new)"
			}
		}
		if err := c.config.SetAdminGroup(ctx, chat, clubID); err != nil {
			return "", err
		}
		next := ""
		if cfg.AnnounceGroup == "" {
			next = "\nNext: type `setup announce` in the players' group."
		}
		return fmt.Sprintf("✅ This is now the *admin group*. Everyone in it can run commands.\n%s%s", clubLine, next), nil
	case "announce":
		if chat == cfg.AdminGroup {
			return "⚠️ This is the admin group. Type `setup announce` in the players' group.", nil
		}
		if err := c.config.SetAnnounceGroup(ctx, chat); err != nil {
			return "", err
		}
		return "✅ This is now the *announcement group*. Lineups and results will be posted here; messages here are ignored by the bot.", nil
	case "status":
		lines := []string{"⚙️ *Bot setup*"}
		if cfg.ClubID != nil {
			if cl, err := c.clubs.GetClub(ctx, *cfg.ClubID); err == nil {
				lines = append(lines, "Club: "+cl.Name)
			}
		} else {
			lines = append(lines, "Club: not set")
		}
		lines = append(lines, "Admin group: "+setOrNot(cfg.AdminGroup), "Announcement group: "+setOrNot(cfg.AnnounceGroup))
		switch chat {
		case cfg.AdminGroup:
			lines = append(lines, "This group: admin group")
		case cfg.AnnounceGroup:
			lines = append(lines, "This group: announcement group")
		default:
			lines = append(lines, "This group: not used by the bot")
		}
		return strings.Join(lines, "\n"), nil
	}
	return "Usage: `setup admin <club name>`, `setup announce`, or `setup status` (type it on the bot's phone).", nil
}

func setOrNot(jid string) string {
	if jid == "" {
		return "not set"
	}
	return "set ✅"
}

// ---------------------------------------------------------------- members

// member lists the members (no text), shows one member (a name), or adds and
// updates members (one "Name [M/F] [A-E]" per line, or one line with a
// gender or grade).
func (c *GroupCommands) member(ctx context.Context, clubID uuid.UUID, rest string) (string, error) {
	if rest == "" {
		return c.memberList(ctx, clubID)
	}
	if !strings.Contains(rest, "\n") {
		if ml := parseMemberLine(rest); ml.Gender == "" && ml.Grade == "" {
			return c.memberInfo(ctx, clubID, ml.Name)
		}
	}
	return c.upsertMembers(ctx, clubID, rest)
}

func (c *GroupCommands) upsertMembers(ctx context.Context, clubID uuid.UUID, rest string) (string, error) {
	added, updated := 0, 0
	var problems []string
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		in, err := memberInput(parseMemberLine(line))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%q: %s", strings.TrimSpace(line), err))
			continue
		}
		_, created, err := c.clubs.UpsertMember(ctx, clubID, in)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%q: %s", strings.TrimSpace(line), userMessage(err)))
			continue
		}
		if created {
			added++
		} else {
			updated++
		}
	}
	reply := fmt.Sprintf("✅ Members updated: %d added, %d updated.", added, updated)
	if updated > 0 {
		reply += "\nExisting members keep their earned rating; use `grade <name> <A-E>` to reset one."
	}
	if len(problems) > 0 {
		reply += "\n⚠️ Skipped:\n• " + strings.Join(problems, "\n• ")
	}
	return reply, nil
}

func memberInput(ml memberLine) (club.MemberInput, error) {
	if ml.Name == "" {
		return club.MemberInput{}, errors.New("missing name")
	}
	in := club.MemberInput{Name: ml.Name}
	if ml.Gender != "" {
		g := player.Male
		if ml.Gender == "F" {
			g = player.Female
		}
		in.Gender = &g
	}
	if ml.Grade != "" {
		g, ok := club.ParseGrade(ml.Grade)
		if !ok {
			return club.MemberInput{}, errors.New("grade must be A–E")
		}
		in.Grade = &g
	}
	return in, nil
}

// grade shows a member's grade (`grade Bob`) or changes it and resets their
// rating to it (`grade Bob A`).
func (c *GroupCommands) grade(ctx context.Context, clubID uuid.UUID, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `grade <name> <A-E>` (resets the rating to the grade's starting rating).", nil
	}
	last := args[len(args)-1]
	if len(args) == 1 || len([]rune(last)) != 1 {
		return c.memberInfo(ctx, clubID, strings.Join(args, " "))
	}
	g, ok := club.ParseGrade(last)
	if !ok {
		return "⚠️ Grade must be A, B, C, D or E.", nil
	}
	name := strings.Join(args[:len(args)-1], " ")
	entry, err := c.clubs.SetGrade(ctx, clubID, name, g)
	if err != nil {
		if appErr, ok := apperr.As(err); ok && appErr.Code == apperr.CodeNotFound {
			return notAMember(name), nil
		}
		return "", err
	}
	return fmt.Sprintf("✅ %s is now grade %s (rating reset to %.0f).", entry.DisplayName, g, entry.Rating), nil
}

func (c *GroupCommands) memberList(ctx context.Context, clubID uuid.UUID) (string, error) {
	roster, err := c.clubs.Roster(ctx, clubID)
	if err != nil {
		return "", err
	}
	if len(roster) == 0 {
		return "ℹ️ No members yet. Add them with `member` followed by one `Name M/F Grade` per line.", nil
	}
	lines := []string{fmt.Sprintf("👥 *Members* (%d)", len(roster))}
	for _, e := range roster {
		lines = append(lines, memberLineText(e))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *GroupCommands) memberInfo(ctx context.Context, clubID uuid.UUID, name string) (string, error) {
	roster, err := c.clubs.Roster(ctx, clubID)
	if err != nil {
		return "", err
	}
	for _, e := range roster {
		if strings.EqualFold(e.DisplayName, name) {
			return memberLineText(e), nil
		}
	}
	return notAMember(name), nil
}

func memberLineText(e club.RosterEntry) string {
	return fmt.Sprintf("• %s (%s) — %s · %.0f", e.DisplayName, genderLetter(e.Gender), gradeOrDash(e.Grade), e.Rating)
}

func notAMember(name string) string {
	return fmt.Sprintf("⚠️ %s is not a member. Add with `in %s M/F <grade>`.", name, name)
}

// ---------------------------------------------------------------- session players

// checkIn puts players in the waiting queue. Members are found by name; an
// entry with a gender or grade ("Eko M C") also creates or updates the
// member. Unknown names without either are reported, not created, so a typo
// never makes a new member.
func (c *GroupCommands) checkIn(ctx context.Context, clubID uuid.UUID, sess session.Session, cmd Command) (string, error) {
	var entries []memberLine
	if cmd.Single {
		entries = []memberLine{parseMemberLine(cmd.Rest)}
	} else {
		entries = parseInEntries(cmd.Rest)
	}
	var in, created, onCourt, unknown, problems []string
	for _, ml := range entries {
		var entry club.RosterEntry
		if ml.Gender == "" && ml.Grade == "" {
			p, err := c.clubs.FindMemberByName(ctx, clubID, ml.Name)
			if err != nil {
				unknown = append(unknown, ml.Name)
				continue
			}
			entry = club.RosterEntry{PlayerID: p.ID, DisplayName: p.DisplayName, Gender: p.Gender}
		} else {
			mi, err := memberInput(ml)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %s", ml.Name, err))
				continue
			}
			e, isNew, err := c.clubs.UpsertMember(ctx, clubID, mi)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %s", ml.Name, userMessage(err)))
				continue
			}
			if isNew {
				created = append(created, fmt.Sprintf("%s (%s)", e.DisplayName, gradeOrDash(e.Grade)))
			}
			entry = e
		}
		sp, err := c.addToSession(ctx, sess, entry)
		if err != nil {
			return "", err
		}
		if sp.Status == session.PlayerPlaying {
			onCourt = append(onCourt, entry.DisplayName)
			continue
		}
		in = append(in, entry.DisplayName)
	}
	if len(entries) == 0 {
		return "Usage: `in Andi, Budi` (add `M/F <grade>` for a new player: `in Eko M C`).", nil
	}
	var lines []string
	if len(in) > 0 {
		lines = append(lines, fmt.Sprintf("✅ Checked in (%d): %s", len(in), strings.Join(in, ", ")))
	}
	if len(created) > 0 {
		lines = append(lines, "🆕 New members: "+strings.Join(created, ", "))
	}
	if len(onCourt) > 0 {
		lines = append(lines, "ℹ️ Already on court: "+strings.Join(onCourt, ", "))
	}
	if len(unknown) > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ Not members: %s. Add with `in <name> M/F <grade>`.", strings.Join(unknown, ", ")))
	}
	if len(problems) > 0 {
		lines = append(lines, "⚠️ Skipped: "+strings.Join(problems, "; "))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *GroupCommands) addToSession(ctx context.Context, sess session.Session, e club.RosterEntry) (session.Player, error) {
	added, err := c.sessions.AddPlayers(ctx, session.AddPlayersInput{
		SessionID: sess.ID,
		Players:   []session.AddPlayerInput{{DisplayName: e.DisplayName, Gender: e.Gender}},
	})
	if err != nil {
		return session.Player{}, err
	}
	return added[0], nil
}

// out takes a player out of the rotation (they went home). They can be
// brought back later with `in`.
func (c *GroupCommands) out(ctx context.Context, sess session.Session, rest string) (string, error) {
	name := strings.Join(strings.Fields(rest), " ")
	if name == "" {
		return "Usage: `out <name>`", nil
	}
	sp, p, err := c.findSessionPlayer(ctx, sess, name)
	if err != nil {
		return "", err
	}
	if sp == nil {
		return fmt.Sprintf("⚠️ No player named %q in this session.", name), nil
	}
	switch sp.Status {
	case session.PlayerEnded:
		return fmt.Sprintf("ℹ️ %s is not in the queue.", p.DisplayName), nil
	case session.PlayerPlaying:
		return fmt.Sprintf("⚠️ %s is on court. Finish (`done`) or `cancel` the match first.", p.DisplayName), nil
	}
	if _, err := c.sessions.SetPlayerStatus(ctx, sp.ID, session.PlayerEnded); err != nil {
		return "", err
	}
	return fmt.Sprintf("👋 %s is out of the rotation. Bring them back with `in %s`.", p.DisplayName, p.DisplayName), nil
}

func (c *GroupCommands) findSessionPlayer(ctx context.Context, sess session.Session, name string) (*session.Player, player.Player, error) {
	roster, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return nil, player.Player{}, err
	}
	for i := range roster {
		sp := roster[i]
		if sp.PlayerID == nil {
			continue
		}
		p, err := c.players.GetByID(ctx, *sp.PlayerID)
		if err == nil && strings.EqualFold(p.DisplayName, name) {
			return &sp, p, nil
		}
	}
	return nil, player.Player{}, nil
}

// ---------------------------------------------------------------- courts & matches

// courts lists the courts (no argument) or grows them to n.
func (c *GroupCommands) courts(ctx context.Context, sess session.Session, args []string) (string, error) {
	if len(args) == 0 {
		return c.courtList(ctx, sess)
	}
	total, err := strconv.Atoi(args[0])
	if err != nil || total < 1 || total > 20 {
		return "⚠️ Court count must be between 1 and 20.", nil
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if total < len(courts) {
		return fmt.Sprintf("ℹ️ This session already has %d courts; courts can't be removed.", len(courts)), nil
	}
	for i := len(courts) + 1; i <= total; i++ {
		if _, err := c.sessions.CreateCourt(ctx, sess.ID, fmt.Sprintf("Court %d", i)); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("✅ %d court(s) configured.", total), nil
}

// auto lets the bot fill free courts by itself.
func (c *GroupCommands) auto(ctx context.Context, sess session.Session) (string, error) {
	if _, err := c.sessions.SetAssignmentMode(ctx, sess.ID, session.AssignmentAutomatic); err != nil {
		return "", err
	}
	if _, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, true); err != nil {
		return "", err
	}
	return "🤖 Auto: the bot fills free courts by rating and waiting time.", nil
}

// manual stops the bot filling courts; the host uses `assign` or `next`.
func (c *GroupCommands) manual(ctx context.Context, sess session.Session) (string, error) {
	if _, err := c.sessions.SetAssignmentMode(ctx, sess.ID, session.AssignmentManual); err != nil {
		return "", err
	}
	return "✋ Manual: courts are only filled by `assign` (or `next`). Use `suggest` for balanced teams.", nil
}

func (c *GroupCommands) next(ctx context.Context, sess session.Session) (string, error) {
	if sess.Status != session.StatusActive {
		return "ℹ️ Session has not started yet. Use `start`.", nil
	}
	court, msg, err := c.freeCourt(ctx, sess)
	if court == nil {
		return msg, err
	}
	_, err = c.matches.GenerateAutomatic(ctx, match.GenerateInput{SessionID: sess.ID, CourtID: court.ID, Format: match.AutoFillFormat})
	if err != nil {
		return "⚠️ Could not assign a match: " + userMessage(err), nil
	}
	return fmt.Sprintf("🏸 %s assigned. The lineup is posted in the announcement group.", court.Name), nil
}

func (c *GroupCommands) freeCourt(ctx context.Context, sess session.Session) (*session.Court, string, error) {
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return nil, "", err
	}
	if len(courts) == 0 {
		return nil, "⚠️ No courts yet. Use `courts <number>`.", nil
	}
	for i := range courts {
		if courts[i].Status == session.CourtAvailable {
			return &courts[i], "", nil
		}
	}
	return nil, "⚠️ No court is available. Finish a match with `done <court> <score>`.", nil
}

func (c *GroupCommands) courtByNumber(ctx context.Context, sess session.Session, n int) (*session.Court, error) {
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return nil, err
	}
	want := fmt.Sprintf("Court %d", n)
	for i := range courts {
		if strings.EqualFold(courts[i].Name, want) {
			return &courts[i], nil
		}
	}
	return nil, nil
}

// resolvePlayers maps names to club members, reporting unknown names.
func (c *GroupCommands) resolvePlayers(ctx context.Context, clubID uuid.UUID, names []string) ([]player.Player, []string) {
	var found []player.Player
	var unknown []string
	for _, n := range names {
		p, err := c.clubs.FindMemberByName(ctx, clubID, n)
		if err != nil {
			unknown = append(unknown, n)
			continue
		}
		found = append(found, p)
	}
	return found, unknown
}

// inferFormat picks the match format from the four players' genders.
func inferFormat(ps []player.Player) match.Format {
	men := 0
	for _, p := range ps {
		if p.Gender == player.Male {
			men++
		}
	}
	switch men {
	case len(ps):
		return match.MenDoubles
	case 0:
		return match.WomenDoubles
	}
	return match.MixedDoubles
}

func (c *GroupCommands) suggest(ctx context.Context, clubID uuid.UUID, sess session.Session, rest string) (string, error) {
	names := splitNames(rest)
	if len(names) != 4 {
		return "Usage: `suggest Alice Bob Cara Dan` (four waiting players).", nil
	}
	ps, unknown := c.resolvePlayers(ctx, clubID, names)
	if len(unknown) > 0 {
		return fmt.Sprintf("⚠️ Not members: %s.", strings.Join(unknown, ", ")), nil
	}
	ids := [4]uuid.UUID{ps[0].ID, ps[1].ID, ps[2].ID, ps[3].ID}
	prop, err := c.matches.RecommendManual(ctx, match.RecommendInput{SessionID: sess.ID, PlayerIDs: ids, Format: inferFormat(ps)})
	if err != nil {
		return "⚠️ " + userMessage(err), nil
	}
	name := make(map[uuid.UUID]string, 4)
	for _, p := range ps {
		name[p.ID] = p.DisplayName
	}
	a1, a2, b1, b2 := name[prop.TeamA[0]], name[prop.TeamA[1]], name[prop.TeamB[0]], name[prop.TeamB[1]]
	return fmt.Sprintf("⚖️ Most balanced: *%s / %s* vs *%s / %s* (avg %.0f vs %.0f)\nTo play it: `assign <court> %s %s vs %s %s`",
		a1, a2, b1, b2, prop.TeamARating, prop.TeamBRating, a1, a2, b1, b2), nil
}

func (c *GroupCommands) assign(ctx context.Context, clubID uuid.UUID, sess session.Session, rest string) (string, error) {
	if sess.Status != session.StatusActive {
		return "ℹ️ Session has not started yet. Use `start`.", nil
	}
	courtNo, teamA, teamB, ok := parseAssign(rest)
	if !ok {
		return "Usage: `assign <court> Alice Bob vs Cara Dan` (commas for names with spaces).", nil
	}
	court, err := c.courtByNumber(ctx, sess, courtNo)
	if err != nil {
		return "", err
	}
	if court == nil {
		return fmt.Sprintf("⚠️ There is no Court %d.", courtNo), nil
	}
	ps, unknown := c.resolvePlayers(ctx, clubID, []string{teamA[0], teamA[1], teamB[0], teamB[1]})
	if len(unknown) > 0 {
		return fmt.Sprintf("⚠️ Not members: %s.", strings.Join(unknown, ", ")), nil
	}
	_, err = c.matches.ConfirmManual(ctx, match.ConfirmManualInput{
		SessionID: sess.ID, CourtID: court.ID, Format: inferFormat(ps),
		TeamA: [2]uuid.UUID{ps[0].ID, ps[1].ID}, TeamB: [2]uuid.UUID{ps[2].ID, ps[3].ID},
	})
	if err != nil {
		return "⚠️ Could not assign: " + userMessage(err), nil
	}
	return fmt.Sprintf("✅ %s: %s / %s vs %s / %s. The lineup is posted in the announcement group.",
		court.Name, ps[0].DisplayName, ps[1].DisplayName, ps[2].DisplayName, ps[3].DisplayName), nil
}

// done records the result of the match on a court: it starts the match if
// it was only assigned, finishes it with the score (updating ratings), frees
// the court and posts the result to the announcement group.
func (c *GroupCommands) done(ctx context.Context, req Request, sess session.Session, args []string) (string, error) {
	courtNo, scoreA, scoreB, _ := parseDone(args)
	if scoreA == scoreB {
		return "⚠️ A match cannot end in a tie.", nil
	}
	court, err := c.courtByNumber(ctx, sess, courtNo)
	if err != nil {
		return "", err
	}
	if court == nil {
		return fmt.Sprintf("⚠️ There is no Court %d.", courtNo), nil
	}
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	for _, m := range matches {
		if m.CourtID != court.ID || (m.Status != match.StatusCreated && m.Status != match.StatusPlaying) {
			continue
		}
		if m.Status == match.StatusCreated {
			if _, err := c.matches.StartMatch(ctx, m.ID); err != nil {
				return "⚠️ Could not finish the match: " + userMessage(err), nil
			}
		}
		if _, err := c.matches.FinishMatch(ctx, match.FinishInput{MatchID: m.ID, ScoreA: scoreA, ScoreB: scoreB}); err != nil {
			return "⚠️ Could not finish the match: " + userMessage(err), nil
		}
		if line := c.resultLine(ctx, m.ID, scoreA, scoreB); line != "" {
			c.announce(ctx, req, fmt.Sprintf("✅ *%s result*\n%s", court.Name, line))
		}
		return fmt.Sprintf("✅ %s finished %d–%d. Ratings updated and the court is free.", court.Name, scoreA, scoreB), nil
	}
	return fmt.Sprintf("ℹ️ No match is running on %s.", court.Name), nil
}

// resultLine renders "Alice / Bob beat Cara / Dan 21–15" (winners first).
func (c *GroupCommands) resultLine(ctx context.Context, matchID uuid.UUID, scoreA, scoreB int32) string {
	mps, err := c.matches.ListPlayers(ctx, matchID)
	if err != nil {
		return ""
	}
	var teamA, teamB []string
	for _, mp := range mps {
		if mp.PlayerID == nil {
			continue
		}
		p, err := c.players.GetByID(ctx, *mp.PlayerID)
		if err != nil {
			continue
		}
		if mp.Team == match.TeamA {
			teamA = append(teamA, p.DisplayName)
		} else {
			teamB = append(teamB, p.DisplayName)
		}
	}
	winners, losers, hi, lo := teamA, teamB, scoreA, scoreB
	if scoreB > scoreA {
		winners, losers, hi, lo = teamB, teamA, scoreB, scoreA
	}
	return fmt.Sprintf("%s beat %s %d–%d", strings.Join(winners, " / "), strings.Join(losers, " / "), hi, lo)
}

func (c *GroupCommands) cancel(ctx context.Context, sess session.Session, args []string) (string, error) {
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if len(args) > 0 {
		courtNo, err := strconv.Atoi(args[0])
		if err != nil || courtNo < 1 {
			return "Usage: `cancel [court]`", nil
		}
		court, err := c.courtByNumber(ctx, sess, courtNo)
		if err != nil {
			return "", err
		}
		if court == nil {
			return fmt.Sprintf("⚠️ There is no Court %d.", courtNo), nil
		}
		for _, m := range matches {
			if m.CourtID == court.ID && (m.Status == match.StatusCreated || m.Status == match.StatusPlaying) {
				return result(fmt.Sprintf("↩️ %s match cancelled. Its players are back in the queue.", court.Name), c.matches.CancelMatch(ctx, m.ID))
			}
		}
		return fmt.Sprintf("ℹ️ No active match to cancel on %s.", court.Name), nil
	}
	// ListBySession is newest first, so this cancels the latest match.
	for _, m := range matches {
		if m.Status == match.StatusCreated || m.Status == match.StatusPlaying {
			return result("↩️ Latest match cancelled. Its players are back in the queue.", c.matches.CancelMatch(ctx, m.ID))
		}
	}
	return "ℹ️ No active match to cancel.", nil
}

// ---------------------------------------------------------------- read-only

func (c *GroupCommands) currentSession(ctx context.Context, clubID uuid.UUID) (session.Session, error) {
	all, err := c.sessions.ListByClub(ctx, clubID)
	if err != nil {
		return session.Session{}, err
	}
	for _, sess := range all {
		if sess.Status == session.StatusActive || sess.Status == session.StatusNotStarted {
			return sess, nil
		}
	}
	return session.Session{}, session.ErrNotFound
}

// status shows the whole session: courts with their lineups, who is
// waiting, and the latest results.
func (c *GroupCommands) status(ctx context.Context, sess session.Session) (string, error) {
	sps, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}

	mode := "auto"
	if sess.AssignmentMode == session.AssignmentManual {
		mode = "manual"
	} else if !sess.AutoFillEnabled {
		mode = "auto (paused)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🏸 *Session* — %s · %s\n", strings.ToLower(string(sess.Status)), mode)

	b.WriteString("\n*Courts*\n")
	if len(courts) == 0 {
		b.WriteString("None yet. Use `courts <n>`.\n")
	}
	lineups := make(map[uuid.UUID]string)
	finished := 0
	var results []string
	for _, m := range matches {
		switch m.Status {
		case match.StatusCreated, match.StatusPlaying:
			if _, ok := lineups[m.CourtID]; !ok {
				lineups[m.CourtID] = c.lineup(ctx, m.ID)
			}
		case match.StatusFinished:
			finished++
			if len(results) < 3 && m.ScoreA != nil && m.ScoreB != nil {
				if line := c.resultLine(ctx, m.ID, *m.ScoreA, *m.ScoreB); line != "" {
					results = append(results, line)
				}
			}
		}
	}
	for _, ct := range courts {
		if l, ok := lineups[ct.ID]; ok {
			fmt.Fprintf(&b, "%s: %s\n", ct.Name, l)
		} else {
			fmt.Fprintf(&b, "%s: free\n", ct.Name)
		}
	}

	var waiting []string
	for _, sp := range sps {
		if sp.Status != session.PlayerWaiting || sp.PlayerID == nil {
			continue
		}
		if p, err := c.players.GetByID(ctx, *sp.PlayerID); err == nil {
			waiting = append(waiting, p.DisplayName)
		}
	}
	fmt.Fprintf(&b, "\n*Waiting (%d)*\n", len(waiting))
	if len(waiting) == 0 {
		b.WriteString("Nobody\n")
	} else {
		b.WriteString(strings.Join(waiting, ", ") + "\n")
	}

	fmt.Fprintf(&b, "\n*Finished (%d)*", finished)
	for _, r := range results {
		b.WriteString("\n• " + r)
	}
	return b.String(), nil
}

// lineup renders "Alice / Bob vs Cara / Dan" for a match.
func (c *GroupCommands) lineup(ctx context.Context, matchID uuid.UUID) string {
	mps, err := c.matches.ListPlayers(ctx, matchID)
	if err != nil {
		return "in use"
	}
	var teamA, teamB []string
	for _, mp := range mps {
		if mp.PlayerID == nil {
			continue
		}
		p, err := c.players.GetByID(ctx, *mp.PlayerID)
		if err != nil {
			continue
		}
		if mp.Team == match.TeamA {
			teamA = append(teamA, p.DisplayName)
		} else {
			teamB = append(teamB, p.DisplayName)
		}
	}
	return strings.Join(teamA, " / ") + " vs " + strings.Join(teamB, " / ")
}

func (c *GroupCommands) courtList(ctx context.Context, sess session.Session) (string, error) {
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if len(courts) == 0 {
		return "ℹ️ No courts yet. Use `courts <n>`.", nil
	}
	lines := []string{fmt.Sprintf("🏟️ *Courts* (%d)", len(courts))}
	for _, ct := range courts {
		state := "free"
		if ct.Status != session.CourtAvailable {
			state = "in use"
		}
		lines = append(lines, fmt.Sprintf("• %s — %s", ct.Name, state))
	}
	return strings.Join(lines, "\n"), nil
}

// ---------------------------------------------------------------- recap

type recapRow struct {
	name          string
	games, w, l   int32
	start, end    float64
	grade         *club.Grade
	suggest       club.Grade
	playedAnyGame bool
}

// recap summarizes a session for the host: games, wins/losses and rating
// movement per player, flagging players whose rating now fits another grade.
func (c *GroupCommands) recap(ctx context.Context, clubID uuid.UUID, sess session.Session) (string, error) {
	sps, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	ranges, err := c.reads.SessionRatingSummary(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	roster, err := c.clubs.Roster(ctx, clubID)
	if err != nil {
		return "", err
	}
	byPlayer := make(map[uuid.UUID]club.RosterEntry, len(roster))
	for _, e := range roster {
		byPlayer[e.PlayerID] = e
	}
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	finished := 0
	for _, m := range matches {
		if m.Status == match.StatusFinished {
			finished++
		}
	}

	rows := make([]recapRow, 0, len(sps))
	for _, sp := range sps {
		if sp.PlayerID == nil {
			continue
		}
		e, ok := byPlayer[*sp.PlayerID]
		if !ok {
			p, err := c.players.GetByID(ctx, *sp.PlayerID)
			if err != nil {
				continue
			}
			e = club.RosterEntry{DisplayName: p.DisplayName}
		}
		r := recapRow{name: e.DisplayName, games: sp.MatchesPlayed, w: sp.Wins, l: sp.Losses, grade: e.Grade, start: e.Rating, end: e.Rating}
		if rr, ok := ranges[*sp.PlayerID]; ok {
			r.start, r.end, r.playedAnyGame = rr.Start, rr.End, true
		}
		r.suggest = club.SuggestGrade(r.end)
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].playedAnyGame != rows[j].playedAnyGame {
			return rows[i].playedAnyGame
		}
		return rows[i].end-rows[i].start > rows[j].end-rows[j].start
	})

	date := time.Now()
	if sess.StartedAt != nil {
		date = *sess.StartedAt
	}
	var b strings.Builder
	plural := "es"
	if finished == 1 {
		plural = ""
	}
	fmt.Fprintf(&b, "📊 *Session recap* — %s · %d match%s\n", date.Format("2 Jan"), finished, plural)
	if len(rows) == 0 {
		b.WriteString("No players took part.")
		return b.String(), nil
	}
	b.WriteString("```\n")
	b.WriteString("Player     G W-L  Rating     Grd\n")
	flagged := false
	for _, r := range rows {
		grade := gradeOrDash(r.grade)
		if r.playedAnyGame && (r.grade == nil || *r.grade != r.suggest) {
			grade += "→" + string(r.suggest)
			flagged = true
		}
		rating := fmt.Sprintf("%4.0f", r.end)
		if r.playedAnyGame {
			rating = fmt.Sprintf("%4.0f%+4.0f", r.end, r.end-r.start)
		}
		fmt.Fprintf(&b, "%-10s %d %d-%d %-10s %s\n", truncate(r.name, 10), r.games, r.w, r.l, rating, grade)
	}
	b.WriteString("```")
	if flagged {
		b.WriteString("\n→ = rating now fits another grade. Change with `grade <name> <A-E>`.")
	}
	return b.String(), nil
}

// setPlayers swaps two players in the session. Either may be waiting or on
// any court (same or different); removed players return to the waiting queue.
func (c *GroupCommands) setPlayers(ctx context.Context, sess session.Session, rest string) (string, error) {
	names := splitNames(rest)
	if len(names) != 2 {
		return "Usage: `set <name1> <name2>` (swap a player on court with someone waiting, or swap two players).", nil
	}
	sp1, p1, err := c.findSessionPlayer(ctx, sess, names[0])
	if err != nil {
		return "", err
	}
	if sp1 == nil || p1.ID == uuid.Nil {
		return fmt.Sprintf("⚠️ No player named %q in this session.", names[0]), nil
	}
	sp2, p2, err := c.findSessionPlayer(ctx, sess, names[1])
	if err != nil {
		return "", err
	}
	if sp2 == nil || p2.ID == uuid.Nil {
		return fmt.Sprintf("⚠️ No player named %q in this session.", names[1]), nil
	}
	if p1.ID == p2.ID {
		return "⚠️ Choose two different players.", nil
	}
	if sp1.Status == session.PlayerEnded || sp2.Status == session.PlayerEnded {
		return "⚠️ Both players must be in the rotation (use `in` to bring someone back).", nil
	}
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	active := make([]match.Match, 0)
	for _, m := range matches {
		if m.Status == match.StatusCreated || m.Status == match.StatusPlaying {
			active = append(active, m)
		}
	}
	type loc struct {
		match *match.Match
		teamA [2]uuid.UUID
		teamB [2]uuid.UUID
	}
	findLoc := func(pid uuid.UUID) (*loc, error) {
		for i := range active {
			m := &active[i]
			mps, err := c.matches.ListPlayers(ctx, m.ID)
			if err != nil {
				return nil, err
			}
			var a, b []uuid.UUID
			for _, mp := range mps {
				if mp.PlayerID == nil {
					continue
				}
				if mp.Team == match.TeamA {
					a = append(a, *mp.PlayerID)
				} else {
					b = append(b, *mp.PlayerID)
				}
			}
			if len(a) == 2 && len(b) == 2 {
				if a[0] == pid || a[1] == pid || b[0] == pid || b[1] == pid {
					return &loc{match: m, teamA: [2]uuid.UUID{a[0], a[1]}, teamB: [2]uuid.UUID{b[0], b[1]}}, nil
				}
			}
		}
		return nil, nil
	}
	l1, err := findLoc(p1.ID)
	if err != nil {
		return "", err
	}
	l2, err := findLoc(p2.ID)
	if err != nil {
		return "", err
	}

	replaceIn := func(l *loc, out, in uuid.UUID) ([2]uuid.UUID, [2]uuid.UUID) {
		a, b := l.teamA, l.teamB
		for i := 0; i < 2; i++ {
			if a[i] == out {
				a[i] = in
			}
			if b[i] == out {
				b[i] = in
			}
		}
		return a, b
	}

	switch {
	case l1 == nil && l2 == nil:
		return "ℹ️ Both players are already waiting.", nil
	case l1 != nil && l2 == nil:
		a, b := replaceIn(l1, p1.ID, p2.ID)
		_, err := c.matches.UpdateRoster(ctx, match.UpdateRosterInput{
			MatchID:               l1.match.ID,
			TeamA:                 a,
			TeamB:                 b,
			RemovedPlayerStatuses: map[uuid.UUID]db.SessionPlayerStatus{p1.ID: db.SessionPlayerStatusWAITING},
		})
		if err != nil {
			return "⚠️ Could not move player: " + userMessage(err), nil
		}
		return fmt.Sprintf("✅ %s is now playing instead of %s (back in queue).", p2.DisplayName, p1.DisplayName), nil
	case l1 == nil && l2 != nil:
		a, b := replaceIn(l2, p2.ID, p1.ID)
		_, err := c.matches.UpdateRoster(ctx, match.UpdateRosterInput{
			MatchID:               l2.match.ID,
			TeamA:                 a,
			TeamB:                 b,
			RemovedPlayerStatuses: map[uuid.UUID]db.SessionPlayerStatus{p2.ID: db.SessionPlayerStatusWAITING},
		})
		if err != nil {
			return "⚠️ Could not move player: " + userMessage(err), nil
		}
		return fmt.Sprintf("✅ %s is now playing instead of %s (back in queue).", p1.DisplayName, p2.DisplayName), nil
	case l1.match.ID == l2.match.ID:
		// swap within same match (teams/positions)
		a, b := l1.teamA, l1.teamB
		swapID := func(x [2]uuid.UUID) [2]uuid.UUID {
			for i := 0; i < 2; i++ {
				if x[i] == p1.ID {
					x[i] = p2.ID
				} else if x[i] == p2.ID {
					x[i] = p1.ID
				}
			}
			return x
		}
		a, b = swapID(a), swapID(b)
		_, err := c.matches.UpdateRoster(ctx, match.UpdateRosterInput{MatchID: l1.match.ID, TeamA: a, TeamB: b})
		if err != nil {
			return "⚠️ Could not swap players: " + userMessage(err), nil
		}
		return fmt.Sprintf("✅ Swapped %s and %s.", p1.DisplayName, p2.DisplayName), nil
	default:
		a1, b1 := replaceIn(l1, p1.ID, p2.ID)
		a2, b2 := replaceIn(l2, p2.ID, p1.ID)
		if _, err := c.matches.UpdateRoster(ctx, match.UpdateRosterInput{MatchID: l1.match.ID, TeamA: a1, TeamB: b1}); err != nil {
			return "⚠️ Could not swap players: " + userMessage(err), nil
		}
		if _, err := c.matches.UpdateRoster(ctx, match.UpdateRosterInput{MatchID: l2.match.ID, TeamA: a2, TeamB: b2}); err != nil {
			return "⚠️ Players were partially swapped (one court updated). Check status and adjust manually.", nil
		}
		return fmt.Sprintf("✅ Swapped %s and %s across courts.", p1.DisplayName, p2.DisplayName), nil
	}
}


// ---------------------------------------------------------------- helpers

// announce posts to the announcement group unless the command was typed
// there (legacy single-group setup), where the reply is already visible.
func (c *GroupCommands) announce(ctx context.Context, req Request, text string) {
	cfg, err := c.config.Get(ctx)
	if err != nil || cfg.AnnounceGroup == "" || cfg.AnnounceGroup == req.Chat.ToNonAD().String() || c.announcer == nil {
		return
	}
	_ = c.announcer.Announce(ctx, text)
}

func genderLetter(g player.Gender) string {
	if g == player.Female {
		return "F"
	}
	return "M"
}

func gradeOrDash(g *club.Grade) string {
	if g == nil {
		return "-"
	}
	return string(*g)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func result(message string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return message, nil
}

// userMessage keeps internal error details out of the chat.
func userMessage(err error) string {
	if appErr, ok := apperr.As(err); ok && appErr.Code != apperr.CodeInternal {
		return appErr.Message
	}
	return "unexpected error"
}

// chat uses Gemini AI to interpret natural language commands from the host.
func (c *GroupCommands) chat(ctx context.Context, req Request, clubID uuid.UUID, sess *session.Session, prompt string) (string, error) {
	if prompt == "" {
		return "🏸 *Deuce AI Chat*\nTell me what to do in natural language, for example:\n• `chat court 1 finished 21-15`\n• `chat check in Alice, Bob, and Dan`\n• `chat start session with 2 courts`\n• `chat who is playing right now?`", nil
	}
	if c.gemini == nil || !c.gemini.IsConfigured() {
		return "⚠️ Gemini API is not configured. Set GEMINI_API_KEY in .env to use natural language commands.", nil
	}

	in := gemini.InterpretInput{
		Prompt: prompt,
	}

	// Fetch registered club members
	if roster, err := c.clubs.Roster(ctx, clubID); err == nil {
		for _, r := range roster {
			in.ClubMembers = append(in.ClubMembers, r.DisplayName)
		}
	}

	if sess != nil {
		in.SessionActive = sess.Status == session.StatusActive

		// List courts
		courts, err := c.sessions.ListCourts(ctx, sess.ID)
		if err == nil {
			matches, _ := c.matches.ListBySession(ctx, sess.ID)
			for _, ct := range courts {
				cc := gemini.CourtContext{
					Name:   ct.Name,
					Status: "available",
				}
				for _, m := range matches {
					if m.CourtID == ct.ID && (m.Status == match.StatusPlaying || m.Status == match.StatusCreated) {
						cc.Status = "playing"
						mps, err := c.matches.ListPlayers(ctx, m.ID)
						if err == nil {
							for _, mp := range mps {
								if mp.PlayerID == nil {
									continue
								}
								p, err := c.players.GetByID(ctx, *mp.PlayerID)
								if err == nil {
									if mp.Team == match.TeamA {
										cc.TeamA = append(cc.TeamA, p.DisplayName)
									} else {
										cc.TeamB = append(cc.TeamB, p.DisplayName)
									}
								}
							}
						}
						break
					}
				}
				in.Courts = append(in.Courts, cc)
			}
		}

		// List waiting players
		sps, err := c.sessions.ListPlayers(ctx, sess.ID)
		if err == nil {
			for _, sp := range sps {
				if sp.Status == session.PlayerWaiting && sp.PlayerID != nil {
					p, err := c.players.GetByID(ctx, *sp.PlayerID)
					if err == nil {
						in.Waiting = append(in.Waiting, p.DisplayName)
					}
				}
			}
		}
	}

	out, err := c.gemini.Interpret(ctx, in)
	if err != nil {
		if errors.Is(err, gemini.ErrRateLimitExceeded) {
			return "⏳ AI rate limit reached (free tier limit). Please wait a moment or use direct commands (e.g. `done 1 21-15`, `in Alice`).", nil
		}
		return fmt.Sprintf("⚠️ Could not process AI request: %s", err.Error()), err
	}

	if len(out.Commands) == 0 {
		if out.Reply != "" {
			return out.Reply, nil
		}
		return "ℹ️ No action was taken.", nil
	}

	var results []string
	for _, cmdStr := range out.Commands {
		subReq := Request{Chat: req.Chat, FromMe: req.FromMe, Text: cmdStr}
		reply, err := c.Handle(ctx, subReq)
		if err != nil {
			results = append(results, fmt.Sprintf("⚠️ `%s`: %s", cmdStr, err.Error()))
		} else if reply != "" {
			results = append(results, reply)
		}
	}

	if out.Reply != "" && len(results) > 0 {
		return fmt.Sprintf("🤖 %s\n\n%s", out.Reply, strings.Join(results, "\n\n")), nil
	} else if len(results) > 0 {
		return strings.Join(results, "\n\n"), nil
	}
	return out.Reply, nil
}

func helpText() string {
	return strings.Join([]string{
		"🏸 *Deuce Bot*",
		"",
		"*🤖 AI (Natural Language)*",
		"`chat <anything>` → e.g. `chat court 1 finished 21-15`",
		"",
		"*📅 SESSION*",
		"`start` → start tonight's session",
		"`stop` → end session + recap",
		"`status` → courts, players, scores",
		"",
		"*🙋 PLAYERS*",
		"`in <names>` → check players in",
		"`out <name>` → take a player out",
		"",
		"*🏟️ COURTS*",
		"`courts <n>` → set number of courts",
		"`next` → fill the next free court",
		"`done <court> <score>` → record result (e.g. `done 1 21-15`)",
		"`cancel [court]` → undo a match (latest, or pick a court: `cancel 2`)",
		"`set <name1> <name2>` → swap players (bench↔court, e.g. `set Alice Bob`)",
		"",
		"*🎯 HOST*",
		"`auto` → bot fills courts",
		"`manual` → you pick lineups",
		"`suggest <4 names>` → balanced teams",
		"`assign <court> A B vs C D` → set lineup",
		"",
		"*👥 MEMBER*",
		"`member` → list all members",
		"`member <name>` → one member's info",
		"`grade <name> <A-E>` → change grade",
		"",
		"━━━━━━━━━━━━━━",
		"",
		"*💡 Examples*",
		"`in Andi, Budi, Mary Jane`",
		"`in Eko M C` _(new player: gender, grade)_",
		"`done 1 21-15` _(Team A first)_",
		"`assign 2 Citra Eko vs Andi Budi`",
		"",
		"*✏️ Add members in bulk*",
		"`member` + one per line:",
		"Andi M B",
		"Citra F A",
		"",
		"_Setup (bot phone only):_ `setup admin <club>`, `setup announce`, `setup status`",
	}, "\n")
}
