package club

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"deuce/backend/internal/apperr"
	"deuce/backend/internal/player"
)

// Grade is the host's skill grade for a member, A (strongest) to E.
type Grade string

const (
	GradeA Grade = "A"
	GradeB Grade = "B"
	GradeC Grade = "C"
	GradeD Grade = "D"
	GradeE Grade = "E"
)

// gradeRatings maps a grade to the starting rating it gives a new member.
// C is the system-wide initial rating, so ungraded and C players start
// level; each grade step is 200 points.
var gradeRatings = map[Grade]float64{
	GradeA: 1400, GradeB: 1200, GradeC: 1000, GradeD: 800, GradeE: 600,
}

// ParseGrade accepts "a".."e" in any case.
func ParseGrade(s string) (Grade, bool) {
	g := Grade(strings.ToUpper(strings.TrimSpace(s)))
	_, ok := gradeRatings[g]
	return g, ok
}

// Rating is the starting rating for the grade.
func (g Grade) Rating() float64 { return gradeRatings[g] }

// SuggestGrade returns the grade whose starting rating is closest to the
// given rating (bands are centred on each grade's rating).
func SuggestGrade(rating float64) Grade {
	switch {
	case rating >= 1300:
		return GradeA
	case rating >= 1100:
		return GradeB
	case rating >= 900:
		return GradeC
	case rating >= 700:
		return GradeD
	default:
		return GradeE
	}
}

// RosterEntry is one club member with their grade and current rating.
type RosterEntry struct {
	PlayerID    uuid.UUID
	DisplayName string
	Gender      player.Gender
	Grade       *Grade
	Rating      float64
}

// MemberInput describes a member to add or update. Nil fields are left
// unchanged for existing members and defaulted for new ones (gender MALE,
// no grade, initial rating).
type MemberInput struct {
	Name   string
	Gender *player.Gender
	Grade  *Grade
}

// UpsertMember adds a member to the club or updates an existing one
// (matched by name, case-insensitively). A new member's starting rating
// comes from their grade. An existing member keeps the rating they have
// earned; only their gender and stored grade are updated. Use SetGrade to
// reset a rating to a grade on purpose.
func (s *Service) UpsertMember(ctx context.Context, clubID uuid.UUID, in MemberInput) (RosterEntry, bool, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return RosterEntry{}, false, apperr.Validation("member name is required")
	}
	if in.Gender != nil && !in.Gender.Valid() {
		return RosterEntry{}, false, apperr.Validation("invalid gender")
	}

	existing, err := s.repo.FindMemberPlayerByName(ctx, clubID, name)
	switch {
	case err == nil:
		gender := existing.Gender
		if in.Gender != nil {
			gender = *in.Gender
		}
		p, err := s.players.UpdateProfile(ctx, existing.ID, existing.DisplayName, gender)
		if err != nil {
			return RosterEntry{}, false, apperr.Internal(err)
		}
		if in.Grade != nil {
			if err := s.repo.SetMemberGrade(ctx, clubID, p.ID, in.Grade); err != nil {
				return RosterEntry{}, false, apperr.Internal(err)
			}
		}
		entry, err := s.rosterEntry(ctx, clubID, p.ID)
		return entry, false, err
	case !errors.Is(err, ErrNotFound):
		return RosterEntry{}, false, apperr.Internal(err)
	}

	gender := player.Male
	if in.Gender != nil {
		gender = *in.Gender
	}
	rating := player.InitialRating
	if in.Grade != nil {
		rating = in.Grade.Rating()
	}
	p, err := s.players.Create(ctx, name, gender)
	if err != nil {
		return RosterEntry{}, false, apperr.Internal(err)
	}
	if _, err := s.players.CreateRating(ctx, p.ID, rating); err != nil {
		return RosterEntry{}, false, apperr.Internal(err)
	}
	if _, err := s.repo.AddMember(ctx, clubID, p.ID, RolePlayer); err != nil {
		return RosterEntry{}, false, apperr.Internal(err)
	}
	if in.Grade != nil {
		if err := s.repo.SetMemberGrade(ctx, clubID, p.ID, in.Grade); err != nil {
			return RosterEntry{}, false, apperr.Internal(err)
		}
	}
	return RosterEntry{PlayerID: p.ID, DisplayName: p.DisplayName, Gender: p.Gender, Grade: in.Grade, Rating: rating}, true, nil
}

// SetGrade changes a member's grade and resets their rating to the grade's
// starting rating — the host's explicit re-calibration.
func (s *Service) SetGrade(ctx context.Context, clubID uuid.UUID, name string, grade Grade) (RosterEntry, error) {
	p, err := s.repo.FindMemberPlayerByName(ctx, clubID, strings.TrimSpace(name))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RosterEntry{}, apperr.NotFound("member")
		}
		return RosterEntry{}, apperr.Internal(err)
	}
	if err := s.repo.SetMemberGrade(ctx, clubID, p.ID, &grade); err != nil {
		return RosterEntry{}, apperr.Internal(err)
	}
	if _, err := s.players.UpdateRating(ctx, p.ID, grade.Rating()); err != nil {
		return RosterEntry{}, apperr.Internal(err)
	}
	return s.rosterEntry(ctx, clubID, p.ID)
}

// Roster lists the club's members, strongest first.
func (s *Service) Roster(ctx context.Context, clubID uuid.UUID) ([]RosterEntry, error) {
	entries, err := s.repo.ListRoster(ctx, clubID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return entries, nil
}

// FindMemberByName looks a member up by name, case-insensitively.
func (s *Service) FindMemberByName(ctx context.Context, clubID uuid.UUID, name string) (player.Player, error) {
	p, err := s.repo.FindMemberPlayerByName(ctx, clubID, strings.TrimSpace(name))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return player.Player{}, apperr.NotFound("member")
		}
		return player.Player{}, apperr.Internal(err)
	}
	return p, nil
}

// FindOrCreateByName returns the oldest club with this name, or creates
// one. Clubs created this way get a technical host identity, which the
// roster hides.
func (s *Service) FindOrCreateByName(ctx context.Context, name string) (Club, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Club{}, false, apperr.Validation("club name is required")
	}
	if clubs, err := s.repo.ListByName(ctx, name); err != nil {
		return Club{}, false, apperr.Internal(err)
	} else if len(clubs) > 0 {
		return clubs[0], false, nil
	}
	res, err := s.CreateClub(ctx, CreateClubInput{ClubName: name, HostDisplayName: "Club Host", HostGender: player.Male})
	if err != nil {
		return Club{}, false, err
	}
	return res.Club, true, nil
}

func (s *Service) rosterEntry(ctx context.Context, clubID, playerID uuid.UUID) (RosterEntry, error) {
	entries, err := s.repo.ListRoster(ctx, clubID)
	if err != nil {
		return RosterEntry{}, apperr.Internal(err)
	}
	for _, e := range entries {
		if e.PlayerID == playerID {
			return e, nil
		}
	}
	return RosterEntry{}, apperr.NotFound("member")
}
