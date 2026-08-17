package matchmaking

import "testing"

func TestGenerateMatch_FourEligiblePlayers(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "a", Rating: 1200, Gender: Male, Status: StatusWaiting},
		{PlayerID: "b", Rating: 1150, Gender: Male, Status: StatusWaiting},
		{PlayerID: "c", Rating: 1100, Gender: Male, Status: StatusWaiting},
		{PlayerID: "d", Rating: 1050, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal.RatingDiff != 0 {
		t.Fatalf("expected the balanced split (A+D vs B+C) to have 0 diff, got %v", proposal.RatingDiff)
	}
}

func TestGenerateMatch_InsufficientPlayers(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "a", Rating: 1200, Gender: Male, Status: StatusWaiting},
		{PlayerID: "b", Rating: 1150, Gender: Male, Status: StatusWaiting},
	}
	_, err := GenerateMatch(pool, MenDoubles)
	if err != ErrInsufficientPlayers {
		t.Fatalf("expected ErrInsufficientPlayers, got %v", err)
	}
}

func TestGenerateMatch_MixedDoubles(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "m1", Rating: 1100, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m2", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "f1", Rating: 1050, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f2", Rating: 950, Gender: Female, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MixedDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal.RatingDiff != 0 {
		t.Fatalf("expected perfectly rating-balanced proposal (diff 0), got %v", proposal.RatingDiff)
	}
}

func TestGenerateMatch_MenDoubles_ExcludesFemales(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "m1", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m2", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m3", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m4", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "f1", Rating: 1000, Gender: Female, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, team := range [][2]TeamAssignment{proposal.TeamA, proposal.TeamB} {
		for _, p := range team {
			if p.Gender != Male {
				t.Fatalf("expected only male players in MEN_DOUBLES, got %v", p.Gender)
			}
		}
	}
}

func TestGenerateMatch_WomenDoubles(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "f1", Rating: 1000, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f2", Rating: 1000, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f3", Rating: 1000, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f4", Rating: 1000, Gender: Female, Status: StatusWaiting},
		{PlayerID: "m1", Rating: 1000, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, WomenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, team := range [][2]TeamAssignment{proposal.TeamA, proposal.TeamB} {
		for _, p := range team {
			if p.Gender != Female {
				t.Fatalf("expected only female players in WOMEN_DOUBLES, got %v", p.Gender)
			}
		}
	}
}

func TestGenerateMatch_ExcludesBreakAndEnded(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "a", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "b", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "c", Rating: 1000, Gender: Male, Status: StatusBreak},
		{PlayerID: "d", Rating: 1000, Gender: Male, Status: StatusEnded},
	}
	_, err := GenerateMatch(pool, MenDoubles)
	if err != ErrInsufficientPlayers {
		t.Fatalf("expected ErrInsufficientPlayers because c and d are not WAITING, got %v", err)
	}
}

func TestGenerateMatch_FewerMatchesPrioritized(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "veteran1", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 5},
		{PlayerID: "veteran2", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 5},
		{PlayerID: "veteran3", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 5},
		{PlayerID: "newbie1", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 0},
		{PlayerID: "newbie2", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 0},
		{PlayerID: "newbie3", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 0},
		{PlayerID: "newbie4", Rating: 1000, Gender: Male, Status: StatusWaiting, MatchesPlayed: 0},
	}
	proposal, err := GenerateMatch(pool, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range append(proposal.TeamA[:], proposal.TeamB[:]...) {
		seen[p.PlayerID] = true
	}
	for _, id := range []string{"newbie1", "newbie2", "newbie3", "newbie4"} {
		if !seen[id] {
			t.Fatalf("expected players with fewer matches played to be prioritized, missing %s", id)
		}
	}
}

func TestGenerateMatch_WaitingTimePrioritized(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "short1", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 10},
		{PlayerID: "short2", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 10},
		{PlayerID: "short3", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 10},
		{PlayerID: "long1", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 600},
		{PlayerID: "long2", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 500},
		{PlayerID: "long3", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 400},
		{PlayerID: "long4", Rating: 1000, Gender: Male, Status: StatusWaiting, WaitingSeconds: 300},
	}
	proposal, err := GenerateMatch(pool, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range append(proposal.TeamA[:], proposal.TeamB[:]...) {
		seen[p.PlayerID] = true
	}
	for _, id := range []string{"long1", "long2", "long3", "long4"} {
		if !seen[id] {
			t.Fatalf("expected longest-waiting players to be prioritized, missing %s", id)
		}
	}
}

func TestGenerateMatch_BalancedTeamsPreferred(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "A", Rating: 1200, Gender: Male, Status: StatusWaiting},
		{PlayerID: "B", Rating: 1150, Gender: Male, Status: StatusWaiting},
		{PlayerID: "C", Rating: 1100, Gender: Male, Status: StatusWaiting},
		{PlayerID: "D", Rating: 1050, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	teamIDs := map[string]bool{proposal.TeamA[0].PlayerID: true, proposal.TeamA[1].PlayerID: true}
	// A+D vs B+C is the perfectly balanced split (diff 0).
	if !((teamIDs["A"] && teamIDs["D"]) || (teamIDs["B"] && teamIDs["C"])) {
		t.Fatalf("expected balanced split A+D vs B+C, got teamA=%v", teamIDs)
	}
}

func TestGenerateMatch_MixedDoubles_UnevenSplit_ThreeMalesOneFemale(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "m1", Rating: 1200, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m2", Rating: 1100, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m3", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "f1", Rating: 1050, Gender: Female, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MixedDoubles)
	if err != nil {
		t.Fatalf("expected a match to form with 3 males + 1 female, got error: %v", err)
	}

	// With all 4 players selected (pool size == 4), the best rating-balanced
	// split (by exhaustive comparison of the 3 possible pairings) is
	// f1+m2 (avg 1075) vs m1+m3 (avg 1100), diff 25 - which mixes gender
	// into one team (f1+m2) while leaving the other all-male (m1+m3).
	if proposal.RatingDiff != 25 {
		t.Fatalf("expected the closest-rating split to have diff 25, got %v", proposal.RatingDiff)
	}
	teamIDs := map[string]bool{proposal.TeamA[0].PlayerID: true, proposal.TeamA[1].PlayerID: true}
	if !((teamIDs["f1"] && teamIDs["m2"]) || (teamIDs["m1"] && teamIDs["m3"])) {
		t.Fatalf("expected balanced split f1+m2 vs m1+m3, got teamA=%v", teamIDs)
	}
}

func TestGenerateMatch_MixedDoubles_UnevenSplit_OneMaleThreeFemales(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "f1", Rating: 1100, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f2", Rating: 1050, Gender: Female, Status: StatusWaiting},
		{PlayerID: "f3", Rating: 900, Gender: Female, Status: StatusWaiting},
		{PlayerID: "m1", Rating: 1200, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MixedDoubles)
	if err != nil {
		t.Fatalf("expected a match to form with 1 male + 3 females, got error: %v", err)
	}

	// Best split: f1+f2 (avg 1075) vs f3+m1 (avg 1050), diff 25 - mixing
	// gender into one team (f3+m1) while leaving the other all-female
	// (f1+f2).
	if proposal.RatingDiff != 25 {
		t.Fatalf("expected the closest-rating split to have diff 25, got %v", proposal.RatingDiff)
	}
	teamIDs := map[string]bool{proposal.TeamA[0].PlayerID: true, proposal.TeamA[1].PlayerID: true}
	if !((teamIDs["f1"] && teamIDs["f2"]) || (teamIDs["f3"] && teamIDs["m1"])) {
		t.Fatalf("expected balanced split f1+f2 vs f3+m1, got teamA=%v", teamIDs)
	}
}

func TestGenerateMatch_MixedDoubles_OpenGender(t *testing.T) {
	pool := []Candidate{
		{PlayerID: "m1", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m2", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m3", Rating: 1000, Gender: Male, Status: StatusWaiting},
		{PlayerID: "m4", Rating: 1000, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := GenerateMatch(pool, MixedDoubles)
	if err != nil {
		t.Fatalf("expected successful match generation for open gender MixedDoubles, got %v", err)
	}
	if proposal.RatingDiff != 0 {
		t.Fatalf("expected 0 rating diff, got %v", proposal.RatingDiff)
	}
}

func TestRecommendSplit_ManualAssignment(t *testing.T) {
	players := [4]Candidate{
		{PlayerID: "A", Rating: 1200, Gender: Male, Status: StatusWaiting},
		{PlayerID: "B", Rating: 1150, Gender: Male, Status: StatusWaiting},
		{PlayerID: "C", Rating: 1100, Gender: Male, Status: StatusWaiting},
		{PlayerID: "D", Rating: 1050, Gender: Male, Status: StatusWaiting},
	}
	proposal, err := RecommendSplit(players, MenDoubles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal.RatingDiff != 0 {
		t.Fatalf("expected balanced recommendation, got diff %v", proposal.RatingDiff)
	}
}
