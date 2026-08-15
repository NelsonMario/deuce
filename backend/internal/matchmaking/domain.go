// Package matchmaking implements deterministic, rating-aware doubles
// matchmaking. It has no dependency on HTTP or PostgreSQL so it can be unit
// tested in complete isolation. Callers are responsible for supplying only
// players that are currently eligible to be considered (i.e. WAITING).
package matchmaking

import (
	"errors"
	"sort"
)

// Gender mirrors player.Gender without importing the player package, keeping
// this package dependency-free.
type Gender string

const (
	Male   Gender = "MALE"
	Female Gender = "FEMALE"
)

// Status mirrors session.PlayerStatus. Only WAITING players are eligible.
type Status string

const (
	StatusWaiting Status = "WAITING"
	StatusPlaying Status = "PLAYING"
	StatusBreak   Status = "BREAK"
	StatusEnded   Status = "ENDED"
)

// Format is the doubles match format being generated.
type Format string

const (
	MixedDoubles Format = "MIXED_DOUBLES"
	MenDoubles   Format = "MEN_DOUBLES"
	WomenDoubles Format = "WOMEN_DOUBLES"
)

// ErrInsufficientPlayers is returned when there are not enough eligible
// players to form a match for the requested format.
var ErrInsufficientPlayers = errors.New("insufficient eligible players for match format")

// Candidate is an eligible player considered for matchmaking.
type Candidate struct {
	PlayerID       string
	Rating         float64
	Gender         Gender
	Status         Status
	MatchesPlayed  int
	WaitingSeconds float64
}

// TeamAssignment is one player's slot in a proposed match.
type TeamAssignment struct {
	PlayerID string
	Rating   float64
	Gender   Gender
}

// Proposal is a generated (or recommended) 2v2 match.
type Proposal struct {
	Format      Format
	TeamA       [2]TeamAssignment
	TeamB       [2]TeamAssignment
	TeamARating float64
	TeamBRating float64
	RatingDiff  float64
}

// eligibleForFormat reports whether c can participate in the given format at
// all (gender-wise). It does not decide team composition.
func eligibleForFormat(c Candidate, format Format) bool {
	if c.Status != StatusWaiting {
		return false
	}
	switch format {
	case MenDoubles:
		return c.Gender == Male
	case WomenDoubles:
		return c.Gender == Female
	case MixedDoubles:
		return c.Gender == Male || c.Gender == Female
	default:
		return false
	}
}

// priorityLess implements the rotation-priority ordering:
//  1. fewer matches played first
//  2. longer waiting time first
//  3. player ID for deterministic tie-breaking
func priorityLess(a, b Candidate) bool {
	if a.MatchesPlayed != b.MatchesPlayed {
		return a.MatchesPlayed < b.MatchesPlayed
	}
	if a.WaitingSeconds != b.WaitingSeconds {
		return a.WaitingSeconds > b.WaitingSeconds
	}
	return a.PlayerID < b.PlayerID
}

func sortByPriority(cs []Candidate) {
	sort.SliceStable(cs, func(i, j int) bool { return priorityLess(cs[i], cs[j]) })
}

// GenerateMatch selects the four highest-priority eligible players for the
// given format and returns the rating-balanced team split.
//
// V1 keeps candidate-group selection simple: it does not brute-force every
// combination of eligible players, only the top-priority group required by
// the format — the top 4 players by rotation priority, drawn from the
// combined eligible pool regardless of format. For MIXED_DOUBLES, gender is
// only a gate on whether the pool counts as mixed at all (it requires at
// least one male and one female among the eligible players); it no longer
// constrains which 4 players are selected or how they are split into teams.
// Within the selected group of four, every valid 2v2 team split is evaluated
// and the one with the smallest rating difference is chosen, with no regard
// to gender.
func GenerateMatch(pool []Candidate, format Format) (*Proposal, error) {
	eligible := make([]Candidate, 0, len(pool))
	for _, c := range pool {
		if eligibleForFormat(c, format) {
			eligible = append(eligible, c)
		}
	}

	switch format {
	case MenDoubles, WomenDoubles:
		sortByPriority(eligible)
		if len(eligible) < 4 {
			return nil, ErrInsufficientPlayers
		}
		group := eligible[:4]
		return bestBalancedSplit(group, format), nil

	case MixedDoubles:
		var males, females int
		for _, c := range eligible {
			if c.Gender == Male {
				males++
			} else {
				females++
			}
		}
		if males < 1 || females < 1 || len(eligible) < 4 {
			return nil, ErrInsufficientPlayers
		}
		sortByPriority(eligible)
		group := eligible[:4]
		return bestBalancedSplit(group, format), nil

	default:
		return nil, errors.New("unknown match format")
	}
}

func toAssignment(c Candidate) TeamAssignment {
	return TeamAssignment{PlayerID: c.PlayerID, Rating: c.Rating, Gender: c.Gender}
}

func makeProposal(format Format, a1, a2, b1, b2 Candidate) *Proposal {
	teamARating := (a1.Rating + a2.Rating) / 2
	teamBRating := (b1.Rating + b2.Rating) / 2
	diff := teamARating - teamBRating
	if diff < 0 {
		diff = -diff
	}
	return &Proposal{
		Format:      format,
		TeamA:       [2]TeamAssignment{toAssignment(a1), toAssignment(a2)},
		TeamB:       [2]TeamAssignment{toAssignment(b1), toAssignment(b2)},
		TeamARating: teamARating,
		TeamBRating: teamBRating,
		RatingDiff:  diff,
	}
}

// bestBalancedSplit evaluates the 3 possible 2v2 splits of 4 players and
// returns the one with the smallest rating difference between teams. It is
// gender-agnostic: callers decide up front which 4 players form the group
// (e.g. filtered to a single gender for MEN_DOUBLES/WOMEN_DOUBLES, or drawn
// from the combined pool for MIXED_DOUBLES), and this helper balances purely
// on rating, with no regard to how genders fall across the two teams.
func bestBalancedSplit(group []Candidate, format Format) *Proposal {
	p := group[0]
	q := group[1]
	r := group[2]
	s := group[3]

	candidates := []*Proposal{
		makeProposal(format, p, q, r, s),
		makeProposal(format, p, r, q, s),
		makeProposal(format, p, s, q, r),
	}
	return bestByDiff(candidates)
}

func bestByDiff(candidates []*Proposal) *Proposal {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.RatingDiff < best.RatingDiff {
			best = c
		}
	}
	return best
}

// RecommendSplit is used by MANUAL assignment: given exactly four
// host-selected eligible players, recommend the best-balanced team split.
// The host may override the recommendation.
func RecommendSplit(players [4]Candidate, format Format) (*Proposal, error) {
	for _, c := range players {
		if !eligibleForFormat(c, format) {
			return nil, errors.New("selected player is not eligible for the chosen format")
		}
	}

	if format == MixedDoubles {
		var males, females int
		for _, c := range players {
			if c.Gender == Male {
				males++
			} else {
				females++
			}
		}
		if males < 1 || females < 1 {
			return nil, errors.New("mixed doubles requires at least one male and one female player")
		}
	}

	group := players[:]
	return bestBalancedSplit(group, format), nil
}
