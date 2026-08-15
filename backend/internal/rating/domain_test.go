package rating

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestInitialRating(t *testing.T) {
	if InitialRating != 1000 {
		t.Fatalf("expected initial rating 1000, got %v", InitialRating)
	}
}

func TestEqualTeamsExpected(t *testing.T) {
	e := Expected(1000, 1000)
	if !almostEqual(e, 0.5) {
		t.Fatalf("expected 0.5 for equal ratings, got %v", e)
	}
}

func TestApplyMatch_EqualTeams(t *testing.T) {
	m := MatchResult{
		TeamA:    [2]PlayerRef{{"a1", 1000}, {"a2", 1000}},
		TeamB:    [2]PlayerRef{{"b1", 1000}, {"b2", 1000}},
		TeamAWon: true,
	}
	results := ApplyMatch(m)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for _, r := range results {
		if r.PlayerID == "a1" || r.PlayerID == "a2" {
			if !almostEqual(r.RatingChange, 16) {
				t.Errorf("expected winner change +16, got %v", r.RatingChange)
			}
		} else {
			if !almostEqual(r.RatingChange, -16) {
				t.Errorf("expected loser change -16, got %v", r.RatingChange)
			}
		}
	}
}

func TestApplyMatch_FavoriteWins(t *testing.T) {
	m := MatchResult{
		TeamA:    [2]PlayerRef{{"a1", 1200}, {"a2", 1200}},
		TeamB:    [2]PlayerRef{{"b1", 1000}, {"b2", 1000}},
		TeamAWon: true,
	}
	results := ApplyMatch(m)
	for _, r := range results {
		if r.PlayerID == "a1" {
			if r.RatingChange <= 0 || r.RatingChange >= 16 {
				t.Errorf("expected small positive change for favorite win, got %v", r.RatingChange)
			}
		}
		if r.PlayerID == "b1" {
			if r.RatingChange >= 0 {
				t.Errorf("expected negative change for underdog loss, got %v", r.RatingChange)
			}
		}
	}
}

func TestApplyMatch_UnderdogWins(t *testing.T) {
	m := MatchResult{
		TeamA:    [2]PlayerRef{{"a1", 1000}, {"a2", 1000}},
		TeamB:    [2]PlayerRef{{"b1", 1200}, {"b2", 1200}},
		TeamAWon: true,
	}
	results := ApplyMatch(m)
	for _, r := range results {
		if r.PlayerID == "a1" {
			if r.RatingChange <= 16 {
				t.Errorf("expected large positive change for underdog win, got %v", r.RatingChange)
			}
		}
		if r.PlayerID == "b1" {
			if r.RatingChange >= 0 {
				t.Errorf("expected negative change for favorite loss, got %v", r.RatingChange)
			}
		}
	}
}

func TestApplyMatch_MultipleMatches(t *testing.T) {
	aRating, bRating := 1000.0, 1000.0
	for i := 0; i < 3; i++ {
		m := MatchResult{
			TeamA:    [2]PlayerRef{{"a1", aRating}, {"a2", aRating}},
			TeamB:    [2]PlayerRef{{"b1", bRating}, {"b2", bRating}},
			TeamAWon: true,
		}
		results := ApplyMatch(m)
		for _, r := range results {
			if r.PlayerID == "a1" {
				aRating = r.RatingAfter
			}
			if r.PlayerID == "b1" {
				bRating = r.RatingAfter
			}
		}
	}
	if aRating <= 1000 || bRating >= 1000 {
		t.Fatalf("expected repeated wins to raise winner rating and lower loser rating, got a=%v b=%v", aRating, bRating)
	}
}

func TestTeamRating(t *testing.T) {
	if got := TeamRating(1200, 1000); got != 1100 {
		t.Fatalf("expected 1100, got %v", got)
	}
}
