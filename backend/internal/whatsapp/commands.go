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
}

func NewGroupCommands(config ConfigStore, clubs *club.Service, sessions *session.Service, matches *match.Service, reads match.ReadRepository, players player.Repository, announcer Announcer) *GroupCommands {
	return &GroupCommands{config: config, clubs: clubs, sessions: sessions, matches: matches, reads: reads, players: players, announcer: announcer}
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
	case "roster":
		return c.roster(ctx, clubID, cmd.Rest)
	case "grade":
		return c.grade(ctx, clubID, cmd.Args)
	case "members":
		return c.members(ctx, clubID)
	}

	sess, err := c.currentSession(ctx, clubID)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return "⚠️ Could not load the current session.", err
	}
	if errors.Is(err, session.ErrNotFound) {
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
		return "🏸 Session created and started. Next: `courts <n>`, then `checkin` or `add` players.", nil
	}

	switch cmd.Name {
	case "status":
		return c.status(ctx, sess)
	case "players":
		return c.playersList(ctx, sess)
	case "matches":
		return c.matchesList(ctx, sess)
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
	case "pause":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, false)
		return result("⏸️ Automatic matchmaking paused. Use `next` or `assign` to fill courts.", err)
	case "resume":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, true)
		return result("▶️ Automatic matchmaking resumed.", err)
	case "mode":
		return c.mode(ctx, sess, cmd.Args)
	case "next":
		return c.next(ctx, sess)
	case "add":
		return c.add(ctx, clubID, sess, cmd.Rest)
	case "checkin":
		return c.checkin(ctx, clubID, sess, cmd.Rest)
	case "remove":
		return c.remove(ctx, sess, cmd.Rest)
	case "courts":
		return c.courts(ctx, sess, cmd.Args)
	case "suggest":
		return c.suggest(ctx, clubID, sess, cmd.Rest)
	case "assign":
		return c.assign(ctx, clubID, sess, cmd.Rest)
	case "done":
		return c.done(ctx, req, sess, cmd.Args)
	case "cancel":
		return c.cancel(ctx, sess)
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

// ---------------------------------------------------------------- roster

func (c *GroupCommands) roster(ctx context.Context, clubID uuid.UUID, rest string) (string, error) {
	lines := strings.Split(rest, "\n")
	added, updated := 0, 0
	var problems []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ml := parseMemberLine(line)
		in, err := memberInput(ml)
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
	if added+updated+len(problems) == 0 {
		return "Usage: `roster` followed by one member per line:\n```\nroster\nAlice F A\nBob M B\n```\nGrades A–E set the starting rating (A=1400 … E=600).", nil
	}
	reply := fmt.Sprintf("✅ Roster updated: %d added, %d updated.", added, updated)
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

func (c *GroupCommands) grade(ctx context.Context, clubID uuid.UUID, args []string) (string, error) {
	if len(args) < 2 {
		return "Usage: `grade <name> <A-E>` (resets the rating to the grade's starting rating).", nil
	}
	g, ok := club.ParseGrade(args[len(args)-1])
	if !ok {
		return "⚠️ Grade must be A, B, C, D or E.", nil
	}
	name := strings.Join(args[:len(args)-1], " ")
	entry, err := c.clubs.SetGrade(ctx, clubID, name, g)
	if err != nil {
		if appErr, ok := apperr.As(err); ok && appErr.Code == apperr.CodeNotFound {
			return fmt.Sprintf("⚠️ %s is not in the roster. Add them with `roster` or `add`.", name), nil
		}
		return "", err
	}
	return fmt.Sprintf("✅ %s is now grade %s (rating reset to %.0f).", entry.DisplayName, g, entry.Rating), nil
}

func (c *GroupCommands) members(ctx context.Context, clubID uuid.UUID) (string, error) {
	roster, err := c.clubs.Roster(ctx, clubID)
	if err != nil {
		return "", err
	}
	if len(roster) == 0 {
		return "ℹ️ The roster is empty. Import it with `roster`.", nil
	}
	lines := []string{fmt.Sprintf("👥 *Members* (%d)", len(roster))}
	for _, e := range roster {
		lines = append(lines, fmt.Sprintf("• %s (%s) — %s · %.0f", e.DisplayName, genderLetter(e.Gender), gradeOrDash(e.Grade), e.Rating))
	}
	return strings.Join(lines, "\n"), nil
}

// ---------------------------------------------------------------- session players

func (c *GroupCommands) add(ctx context.Context, clubID uuid.UUID, sess session.Session, rest string) (string, error) {
	ml := parseMemberLine(rest)
	in, err := memberInput(ml)
	if err != nil {
		return "Usage: `add <name> [M/F] [A-E]`", nil
	}
	entry, created, err := c.clubs.UpsertMember(ctx, clubID, in)
	if err != nil {
		return "", err
	}
	sp, err := c.addToSession(ctx, sess, entry)
	if err != nil {
		return "", err
	}
	if sp.Status == session.PlayerPlaying {
		return fmt.Sprintf("ℹ️ %s is already on court.", entry.DisplayName), nil
	}
	note := ""
	if created {
		note = fmt.Sprintf(" New member, grade %s.", gradeOrDash(entry.Grade))
	}
	return fmt.Sprintf("✅ %s added to the waiting queue.%s", entry.DisplayName, note), nil
}

func (c *GroupCommands) checkin(ctx context.Context, clubID uuid.UUID, sess session.Session, rest string) (string, error) {
	names := splitNames(rest)
	if len(names) == 0 {
		return "Usage: `checkin Alice Bob Cara` (use commas for names with spaces: `checkin Mary Jane, Bob`).", nil
	}
	var in, unknown []string
	for _, name := range names {
		p, err := c.clubs.FindMemberByName(ctx, clubID, name)
		if err != nil {
			unknown = append(unknown, name)
			continue
		}
		if _, err := c.addToSession(ctx, sess, club.RosterEntry{PlayerID: p.ID, DisplayName: p.DisplayName, Gender: p.Gender}); err != nil {
			return "", err
		}
		in = append(in, p.DisplayName)
	}
	reply := ""
	if len(in) > 0 {
		reply = fmt.Sprintf("✅ Checked in (%d): %s", len(in), strings.Join(in, ", "))
	}
	if len(unknown) > 0 {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("⚠️ Not in the roster: %s. Add with `add <name> M/F <grade>`.", strings.Join(unknown, ", "))
	}
	return reply, nil
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

// remove takes a player out of the rotation (they went home). They can be
// brought back later with `add` or `checkin`.
func (c *GroupCommands) remove(ctx context.Context, sess session.Session, rest string) (string, error) {
	name := strings.Join(strings.Fields(rest), " ")
	if name == "" {
		return "Usage: `remove <name>`", nil
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
	return fmt.Sprintf("👋 %s removed from the queue.", p.DisplayName), nil
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

func (c *GroupCommands) courts(ctx context.Context, sess session.Session, args []string) (string, error) {
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

func (c *GroupCommands) mode(ctx context.Context, sess session.Session, args []string) (string, error) {
	if len(args) != 1 {
		return "Usage: `mode auto` (bot fills courts) or `mode manual` (you `assign` every court).", nil
	}
	switch strings.ToLower(args[0]) {
	case "auto":
		if _, err := c.sessions.SetAssignmentMode(ctx, sess.ID, session.AssignmentAutomatic); err != nil {
			return "", err
		}
		return "🤖 Automatic mode: the bot fills free courts by rating and waiting time.", nil
	case "manual":
		if _, err := c.sessions.SetAssignmentMode(ctx, sess.ID, session.AssignmentManual); err != nil {
			return "", err
		}
		return "✋ Manual mode: courts are only filled by `assign` (or `next`). Use `suggest` for a balanced split.", nil
	}
	return "Usage: `mode auto` or `mode manual`.", nil
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
		return fmt.Sprintf("⚠️ Not in the roster: %s.", strings.Join(unknown, ", ")), nil
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
		return fmt.Sprintf("⚠️ Not in the roster: %s.", strings.Join(unknown, ", ")), nil
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

func (c *GroupCommands) cancel(ctx context.Context, sess session.Session) (string, error) {
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
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

func (c *GroupCommands) status(ctx context.Context, sess session.Session) (string, error) {
	players, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	waiting, playing, inSession := 0, 0, 0
	for _, p := range players {
		switch p.Status {
		case session.PlayerWaiting:
			waiting++
		case session.PlayerPlaying:
			playing++
		}
		if p.Status != session.PlayerEnded {
			inSession++
		}
	}
	mode := "auto"
	if sess.AssignmentMode == session.AssignmentManual {
		mode = "manual"
	} else if !sess.AutoFillEnabled {
		mode = "auto (paused)"
	}
	return fmt.Sprintf("🏸 *Session Status*\n\nStatus: %s\nMode: %s\nCourts: %d\nPlayers: %d\nPlaying: %d\nWaiting: %d", sess.Status, mode, len(courts), inSession, playing, waiting), nil
}

func (c *GroupCommands) playersList(ctx context.Context, sess session.Session) (string, error) {
	roster, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if len(roster) == 0 {
		return "ℹ️ No players yet.", nil
	}
	lines := []string{"👥 *Players*"}
	for _, sp := range roster {
		name := "Unknown player"
		if sp.PlayerID != nil {
			if p, err := c.players.GetByID(ctx, *sp.PlayerID); err == nil {
				name = p.DisplayName
			}
		}
		lines = append(lines, fmt.Sprintf("• %s — %s", name, sp.Status))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *GroupCommands) matchesList(ctx context.Context, sess session.Session) (string, error) {
	matches, err := c.matches.ListBySession(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "ℹ️ No matches yet.", nil
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	courtNames := make(map[uuid.UUID]string, len(courts))
	for _, ct := range courts {
		courtNames[ct.ID] = ct.Name
	}
	lines := []string{"🏸 *Matches*"}
	for _, m := range matches {
		line := fmt.Sprintf("• %s — %s", courtNames[m.CourtID], m.Status)
		if m.ScoreA != nil && m.ScoreB != nil {
			line += fmt.Sprintf(" (%d–%d)", *m.ScoreA, *m.ScoreB)
		}
		lines = append(lines, line)
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

func helpText() string {
	return "🏸 *Deuce Bot*\n\n" +
		"*Session:* `start`, `stop`, `status`, `players`, `matches`\n" +
		"*Courts:* `courts <n>`, `next`, `done <court> <a>-<b>`, `cancel`\n" +
		"*Players:* `checkin <names>`, `add <name> [M/F] [A-E]`, `remove <name>`\n" +
		"*Manual:* `mode auto|manual`, `suggest <4 names>`, `assign <court> A B vs C D`, `pause`, `resume`\n" +
		"*Roster:* `roster` (one member per line), `members`, `grade <name> <A-E>`\n" +
		"*Setup (bot phone):* `setup admin <club>`, `setup announce`, `setup status`"
}
