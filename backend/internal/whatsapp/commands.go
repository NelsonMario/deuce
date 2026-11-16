package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/types"

	"deuce/backend/internal/match"
	"deuce/backend/internal/player"
	"deuce/backend/internal/session"
)

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

func (c *GroupCommands) Handle(ctx context.Context, sender types.JID, input string) (string, error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}
	command := strings.ToLower(parts[0])
	if command == "help" {
		return helpText(c.isAdmin(sender)), nil
	}
	sess, err := c.currentSession(ctx)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return "⚠️ Could not load the current session.", err
	}
	switch command {
	case "status":
		if err != nil {
			return "ℹ️ No session is running.", nil
		}
		return c.status(ctx, sess)
	case "players":
		if err != nil {
			return "ℹ️ No session is running.", nil
		}
		return c.playersList(ctx, sess)
	case "matches":
		if err != nil {
			return "ℹ️ No session is running.", nil
		}
		return c.matchesList(ctx, sess)
	}
	if !c.isAdmin(sender) {
		return "⛔ This command is for group admins only.", nil
	}
	if err != nil && command != "start" {
		return "ℹ️ No session is running. Use `start` to create one.", nil
	}
	switch command {
	case "start":
		if errors.Is(err, session.ErrNotFound) {
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
		_, err := c.sessions.EndSession(ctx, sess.ID)
		return result("🏁 Session finished.", err)
	case "pause":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, false)
		return result("⏸️ Automatic matchmaking paused.", err)
	case "resume":
		_, err := c.sessions.SetAutoFillEnabled(ctx, sess.ID, true)
		return result("▶️ Automatic matchmaking resumed.", err)
	case "next":
		courts, err := c.sessions.ListCourts(ctx, sess.ID)
		if err != nil {
			return "", err
		}
		for _, court := range courts {
			if court.Status == session.CourtAvailable {
				_, err = c.matches.GenerateAutomatic(ctx, match.GenerateInput{SessionID: sess.ID, CourtID: court.ID, Format: match.MixedDoubles})
				if err == nil {
					return "🏸 Match assigned. The lineup will be announced here.", nil
				}
				return "⚠️ Could not assign a match: " + err.Error(), nil
			}
		}
		return "⚠️ No court is available.", nil
	case "add":
		if len(parts) < 2 {
			return "Usage: `add <name> [M/F]`", nil
		}
		gender := player.Male
		nameParts := parts[1:]
		if last := strings.ToUpper(nameParts[len(nameParts)-1]); last == "M" || last == "F" {
			if last == "F" {
				gender = player.Female
			}
			nameParts = nameParts[:len(nameParts)-1]
		}
		name := strings.Join(nameParts, " ")
		if name == "" {
			return "Usage: `add <name> [M/F]`", nil
		}
		_, err := c.sessions.AddPlayers(ctx, session.AddPlayersInput{SessionID: sess.ID, Players: []session.AddPlayerInput{{DisplayName: name, Gender: gender}}})
		return result("✅ Player added to the waiting queue.", err)
	case "courts":
		if len(parts) != 2 {
			return "Usage: `courts <number>`", nil
		}
		total, err := strconv.Atoi(parts[1])
		if err != nil || total < 1 || total > 20 {
			return "⚠️ Court count must be between 1 and 20.", nil
		}
		courts, err := c.sessions.ListCourts(ctx, sess.ID)
		if err != nil {
			return "", err
		}
		for i := len(courts) + 1; i <= total; i++ {
			if _, err := c.sessions.CreateCourt(ctx, sess.ID, fmt.Sprintf("Court %d", i)); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("✅ %d court(s) configured.", total), nil
	case "cancel":
		matches, err := c.matches.ListBySession(ctx, sess.ID)
		if err != nil {
			return "", err
		}
		for _, m := range matches {
			if m.Status == match.StatusCreated || m.Status == match.StatusPlaying {
				return result("↩️ Latest match cancelled.", c.matches.CancelMatch(ctx, m.ID))
			}
		}
		return "ℹ️ No active match to cancel.", nil
	default:
		return "❓ Unknown command. Type `help`.", nil
	}
}

func (c *GroupCommands) isAdmin(sender types.JID) bool {
	_, ok := c.admins[sender.ToNonAD().String()]
	return ok
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
	waiting, playing := 0, 0
	for _, p := range players {
		if p.Status == session.PlayerWaiting {
			waiting++
		}
		if p.Status == session.PlayerPlaying {
			playing++
		}
	}
	return fmt.Sprintf("🏸 *Session Status*\n\nStatus: %s\nCourts: %d\nPlayers: %d\nPlaying: %d\nWaiting: %d", sess.Status, len(courts), len(players), playing, waiting), nil
}

func (c *GroupCommands) playersList(ctx context.Context, sess session.Session) (string, error) {
	roster, err := c.sessions.ListPlayers(ctx, sess.ID)
	if err != nil {
		return "", err
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
	lines := []string{"🏸 *Matches*"}
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("• %s — %s", m.Format, m.Status))
	}
	return strings.Join(lines, "\n"), nil
}

func result(message string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return message, nil
}

func helpText(admin bool) string {
	text := "🏸 *Deuce Group Bot*\n\nPublic: `status`, `players`, `matches`"
	if admin {
		text += "\nAdmin: `start` (creates a session if needed), `stop`, `pause`, `resume`, `next`, `add <name> [M/F]`, `courts <n>`, `cancel`"
	}
	return text
}
