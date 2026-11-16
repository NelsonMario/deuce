package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

// Sender identifies who wrote a group message. WhatsApp groups may address
// members by LID (`123@lid`) instead of phone number; Alt then carries the
// phone-number JID (or vice versa), so admin checks look at both.
type Sender struct {
	JID types.JID
	Alt types.JID
}

// GroupCommands is the application-facing command adapter for one club. It
// authorizes mutation by sender JID; all read commands are public to the
// configured group.
type GroupCommands struct {
	clubID   uuid.UUID
	admins   map[string]struct{}
	sessions *session.Service
	matches  *match.Service
	players  player.Repository
}

func NewGroupCommands(clubID uuid.UUID, adminJIDs []string, sessions *session.Service, matches *match.Service, players player.Repository) *GroupCommands {
	admins := make(map[string]struct{}, len(adminJIDs))
	for _, jid := range adminJIDs {
		admins[strings.TrimSpace(jid)] = struct{}{}
	}
	return &GroupCommands{clubID: clubID, admins: admins, sessions: sessions, matches: matches, players: players}
}

// Handle runs one group message. Messages that are not commands return an
// empty reply so normal group conversation never gets a bot response.
func (c *GroupCommands) Handle(ctx context.Context, sender Sender, input string) (string, error) {
	cmd, ok := ParseCommand(input)
	if !ok {
		return "", nil
	}
	admin := c.isAdmin(sender)
	if cmd.Name == "help" {
		return helpText(admin), nil
	}
	sess, err := c.currentSession(ctx)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return "⚠️ Could not load the current session.", err
	}
	noSession := err != nil
	switch cmd.Name {
	case "status":
		if noSession {
			return "ℹ️ No session is running.", nil
		}
		return c.status(ctx, sess)
	case "players":
		if noSession {
			return "ℹ️ No session is running.", nil
		}
		return c.playersList(ctx, sess)
	case "matches":
		if noSession {
			return "ℹ️ No session is running.", nil
		}
		return c.matchesList(ctx, sess)
	}
	if !admin {
		return "⛔ This command is for group admins only.", nil
	}
	if noSession && cmd.Name != "start" {
		return "ℹ️ No session is running. Use `start` to create one.", nil
	}
	switch cmd.Name {
	case "start":
		if noSession {
			created, createErr := c.sessions.CreateSession(ctx, session.CreateSessionInput{
				ClubID:         c.clubID,
				Name:           "WhatsApp Session",
				AssignmentMode: session.AssignmentAutomatic,
			})
			if createErr != nil {
				return "", createErr
			}
			_, startErr := c.sessions.StartSession(ctx, created.ID)
			return result("🏸 Session created and started.", startErr)
		}
		if sess.Status == session.StatusActive {
			return "ℹ️ Session is already active.", nil
		}
		_, err := c.sessions.StartSession(ctx, sess.ID)
		return result("🏸 Session started.", err)
	case "stop":
		if sess.Status != session.StatusActive {
			return "ℹ️ Session has not started yet.", nil
		}
		_, err := c.sessions.EndSession(ctx, sess.ID)
		return result("🏁 Session finished.", err)
	case "pause":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, false)
		return result("⏸️ Automatic matchmaking paused.", err)
	case "resume":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, true)
		return result("▶️ Automatic matchmaking resumed.", err)
	case "next":
		return c.next(ctx, sess)
	case "add":
		return c.add(ctx, sess, cmd.Args)
	case "remove":
		return c.remove(ctx, sess, cmd.Args)
	case "courts":
		return c.courts(ctx, sess, cmd.Args)
	case "done":
		return c.done(ctx, sess, cmd.Args)
	case "cancel":
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
	return "", nil
}

func (c *GroupCommands) next(ctx context.Context, sess session.Session) (string, error) {
	if sess.Status != session.StatusActive {
		return "ℹ️ Session has not started yet. Use `start`.", nil
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if len(courts) == 0 {
		return "⚠️ No courts yet. Use `courts <number>`.", nil
	}
	for _, court := range courts {
		if court.Status != session.CourtAvailable {
			continue
		}
		_, err = c.matches.GenerateAutomatic(ctx, match.GenerateInput{SessionID: sess.ID, CourtID: court.ID, Format: match.AutoFillFormat})
		if err == nil {
			return "🏸 Match assigned. The lineup will be announced here.", nil
		}
		return "⚠️ Could not assign a match: " + userMessage(err), nil
	}
	return "⚠️ No court is available. Finish a match with `done <court> <score>`.", nil
}

func (c *GroupCommands) add(ctx context.Context, sess session.Session, args []string) (string, error) {
	gender := player.Male
	nameParts := args
	if len(nameParts) > 0 {
		if last := strings.ToUpper(nameParts[len(nameParts)-1]); last == "M" || last == "F" {
			if last == "F" {
				gender = player.Female
			}
			nameParts = nameParts[:len(nameParts)-1]
		}
	}
	name := strings.Join(nameParts, " ")
	if name == "" {
		return "Usage: `add <name> [M/F]`", nil
	}
	added, err := c.sessions.AddPlayers(ctx, session.AddPlayersInput{SessionID: sess.ID, Players: []session.AddPlayerInput{{DisplayName: name, Gender: gender}}})
	if err != nil {
		return "", err
	}
	if len(added) == 1 && added[0].Status == session.PlayerPlaying {
		return fmt.Sprintf("ℹ️ %s is already on court.", name), nil
	}
	return fmt.Sprintf("✅ %s added to the waiting queue.", name), nil
}

// remove takes a player out of the rotation (they went home). They can be
// brought back later with `add`.
func (c *GroupCommands) remove(ctx context.Context, sess session.Session, args []string) (string, error) {
	name := strings.Join(args, " ")
	if name == "" {
		return "Usage: `remove <name>`", nil
	}
	roster, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	for _, sp := range roster {
		if sp.PlayerID == nil {
			continue
		}
		p, err := c.players.GetByID(ctx, *sp.PlayerID)
		if err != nil || !strings.EqualFold(p.DisplayName, name) {
			continue
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
	return fmt.Sprintf("⚠️ No player named %q in this session.", name), nil
}

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

// done records the result of the match on a court: it starts the match if it
// was only assigned, finishes it with the score (updating ratings), and frees
// the court so the next match can be assigned.
func (c *GroupCommands) done(ctx context.Context, sess session.Session, args []string) (string, error) {
	courtNo, scoreA, scoreB, _ := parseDone(args)
	if scoreA == scoreB {
		return "⚠️ A match cannot end in a tie.", nil
	}
	courts, err := c.sessions.ListCourts(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	courtName := fmt.Sprintf("Court %d", courtNo)
	var court *session.Court
	for i := range courts {
		if strings.EqualFold(courts[i].Name, courtName) {
			court = &courts[i]
			break
		}
	}
	if court == nil {
		return fmt.Sprintf("⚠️ There is no %s.", courtName), nil
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
		return fmt.Sprintf("✅ %s finished %d–%d. Ratings updated and the court is free.", court.Name, scoreA, scoreB), nil
	}
	return fmt.Sprintf("ℹ️ No match is running on %s.", court.Name), nil
}

func (c *GroupCommands) isAdmin(sender Sender) bool {
	for _, jid := range []types.JID{sender.JID, sender.Alt} {
		if jid.IsEmpty() {
			continue
		}
		if _, ok := c.admins[jid.ToNonAD().String()]; ok {
			return true
		}
	}
	return false
}

func (c *GroupCommands) currentSession(ctx context.Context) (session.Session, error) {
	all, err := c.sessions.ListByClub(ctx, c.clubID)
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
	return fmt.Sprintf("🏸 *Session Status*\n\nStatus: %s\nCourts: %d\nPlayers: %d\nPlaying: %d\nWaiting: %d", sess.Status, len(courts), inSession, playing, waiting), nil
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

func result(message string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return message, nil
}

// userMessage keeps internal error details out of the group chat.
func userMessage(err error) string {
	if appErr, ok := apperr.As(err); ok && appErr.Code != apperr.CodeInternal {
		return appErr.Message
	}
	return "unexpected error"
}

func helpText(admin bool) string {
	text := "🏸 *Deuce Group Bot*\n\nPublic: `status`, `players`, `matches`"
	if admin {
		text += "\nAdmin: `start`, `stop`, `pause`, `resume`, `next`, `add <name> [M/F]`, `remove <name>`, `courts <n>`, `done <court> <a>-<b>`, `cancel`"
	}
	return text
}
