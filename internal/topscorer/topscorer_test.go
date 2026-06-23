package topscorer

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBlocksWithinWindow(t *testing.T) {
	limiter := &rateLimiter{hits: map[string][]time.Time{}}

	if !limiter.allow("user-1", 2, time.Minute) {
		t.Fatal("first request was blocked")
	}
	if !limiter.allow("user-1", 2, time.Minute) {
		t.Fatal("second request was blocked")
	}
	if limiter.allow("user-1", 2, time.Minute) {
		t.Fatal("third request was not blocked")
	}
}

func TestRateLimiterExpiresOldHits(t *testing.T) {
	limiter := &rateLimiter{hits: map[string][]time.Time{
		"user-1": {time.Now().Add(-2 * time.Minute)},
	}}

	if !limiter.allow("user-1", 1, time.Minute) {
		t.Fatal("expired hit still counted against the rate limit")
	}
}

func TestOrderByGoalsKeepsScorersAheadOfZeroGoalPlayers(t *testing.T) {
	// "Stale" carries a low provider rank but no goals — the exact shape that
	// used to surface a 0-goal player at #1. It must sort to the bottom.
	players := []Player{
		{Name: "Stale", Goals: 0, Rank: 1},
		{Name: "Bravo", Goals: 3, Rank: 0},
		{Name: "Alpha", Goals: 3, Rank: 0},
		{Name: "Charlie", Goals: 5, Rank: 0},
	}

	orderByGoals(players)
	assignCompetitionRanks(players)

	wantOrder := []string{"Charlie", "Alpha", "Bravo", "Stale"}
	for i, want := range wantOrder {
		if players[i].Name != want {
			t.Fatalf("position %d: got %q, want %q (full order %+v)", i, players[i].Name, want, players)
		}
	}
	if players[3].Rank == 1 {
		t.Fatalf("a zero-goal player must never hold rank 1, got %+v", players[3])
	}
}

func TestAssignCompetitionRanksSharesAndSkipsForTies(t *testing.T) {
	// Already ordered by goals desc: 5, 3, 3, 1 -> ranks 1, 2, 2, 4.
	players := []Player{
		{Name: "A", Goals: 5},
		{Name: "B", Goals: 3},
		{Name: "C", Goals: 3},
		{Name: "D", Goals: 1},
	}

	assignCompetitionRanks(players)

	wantRanks := []int{1, 2, 2, 4}
	for i, want := range wantRanks {
		if players[i].Rank != want {
			t.Fatalf("player %q: got rank %d, want %d", players[i].Name, players[i].Rank, want)
		}
	}
}

func TestParseDongqiudiTopScorers(t *testing.T) {
	page := `<table class="cell_data"><tbody>
		<tr><th></th><th class="person">球员</th><th class="team">球队</th><th>进球(点球)</th></tr>
		<tr><td class="rank font-family-number">1</td><td class="person"><img src="x"><span class="name">梅西</span></td><td class="team"> 阿根廷 </td><td class="font-family-number">5</td></tr>
		<tr><td class="rank font-family-number">2</td><td class="person"><img src="x"><span class="name">姆巴佩</span></td><td class="team"> 法国 </td><td class="font-family-number">4</td></tr>
		<tr><td class="rank font-family-number">3</td><td class="person"><img src="x"><span class="name">哈兰德</span></td><td class="team"> 挪威 </td><td class="font-family-number">4</td></tr>
	</tbody></table>`

	players, err := parseDongqiudiTopScorers(page)
	if err != nil {
		t.Fatalf("parseDongqiudiTopScorers() error = %v", err)
	}
	if len(players) != 3 {
		t.Fatalf("len(players) = %d, want 3", len(players))
	}
	want := []struct {
		name  string
		team  string
		goals int
	}{
		{"梅西", "阿根廷", 5},
		{"姆巴佩", "法国", 4},
		{"哈兰德", "挪威", 4},
	}
	for i, row := range want {
		if players[i].Name != row.name || players[i].TeamName != row.team || players[i].Goals != row.goals || players[i].Rank != i+1 {
			t.Fatalf("players[%d] = %+v, want name=%s team=%s goals=%d rank=%d", i, players[i], row.name, row.team, row.goals, i+1)
		}
	}
}

func TestDongqiudiSourceCachesFetches(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`<tr><td class="rank font-family-number">1</td><td class="person"><span class="name">梅西</span></td><td class="team">阿根廷</td><td class="font-family-number">5</td></tr>`))
	}))
	defer server.Close()

	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	source := &dongqiudiTopScorerSource{url: server.URL, http: server.Client(), minFetch: time.Hour, now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		players, err := source.TopScorers(t.Context())
		if err != nil {
			t.Fatalf("TopScorers() error = %v", err)
		}
		if len(players) != 1 || players[0].Name != "梅西" || players[0].Goals != 5 {
			t.Fatalf("TopScorers() = %+v", players)
		}
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
}
