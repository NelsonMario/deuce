// Package rating implements a simple Elo-style rating engine for doubles
// badminton matches. It has no dependency on HTTP or PostgreSQL so it can be
// unit tested in complete isolation.
package rating

import "math"

// InitialRating is the rating every new player starts with.
const InitialRating = 1000.0

// K is the Elo K-factor applied to every rating change.
const K = 32.0

// TeamRating returns the average rating of a two-player doubles team.
func TeamRating(p1, p2 float64) float64 {
	return (p1 + p2) / 2
}

// Expected returns the expected score (win probability) of a team with
// rating `team` against an opponent team with rating `opponent`.
func Expected(team, opponent float64) float64 {
	return 1 / (1 + math.Pow(10, (opponent-team)/400))
}

// Outcome is the actual result of a match from one team's perspective.
type Outcome float64

const (
	Loss Outcome = 0
	Win  Outcome = 1
)

// Change returns the Elo rating delta for a team, given its own rating, the
// opponent team's rating, and the actual outcome (Win/Loss).
func Change(teamRating, opponentRating float64, actual Outcome) float64 {
	expected := Expected(teamRating, opponentRating)
	return K * (float64(actual) - expected)
}

// PlayerResult is the rating outcome for a single player after a match.
type PlayerResult struct {
	PlayerID     string
	RatingBefore float64
	RatingChange float64
	RatingAfter  float64
}

// MatchResult is the full input needed to compute rating changes for a
// doubles match: two players per team and which team won.
type MatchResult struct {
	TeamA    [2]PlayerRef
	TeamB    [2]PlayerRef
	TeamAWon bool
}

// PlayerRef identifies a player and their rating going into the match.
type PlayerRef struct {
	PlayerID string
	Rating   float64
}

// ApplyMatch computes the new ratings for all four players in a doubles
// match. The same rating change is applied to both players on a team.
func ApplyMatch(m MatchResult) []PlayerResult {
	teamARating := TeamRating(m.TeamA[0].Rating, m.TeamA[1].Rating)
	teamBRating := TeamRating(m.TeamB[0].Rating, m.TeamB[1].Rating)

	var teamAOutcome, teamBOutcome Outcome
	if m.TeamAWon {
		teamAOutcome, teamBOutcome = Win, Loss
	} else {
		teamAOutcome, teamBOutcome = Loss, Win
	}

	changeA := Change(teamARating, teamBRating, teamAOutcome)
	changeB := Change(teamBRating, teamARating, teamBOutcome)

	results := make([]PlayerResult, 0, 4)
	for _, p := range m.TeamA {
		results = append(results, PlayerResult{
			PlayerID:     p.PlayerID,
			RatingBefore: p.Rating,
			RatingChange: changeA,
			RatingAfter:  p.Rating + changeA,
		})
	}
	for _, p := range m.TeamB {
		results = append(results, PlayerResult{
			PlayerID:     p.PlayerID,
			RatingBefore: p.Rating,
			RatingChange: changeB,
			RatingAfter:  p.Rating + changeB,
		})
	}
	return results
}
