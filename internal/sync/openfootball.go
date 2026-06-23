package sync

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/oyvhov/world-cup-pool/internal/seed"
)

// openfootball is the free live-results source: the same project we seed
// from publishes scores into 2026/worldcup.json during the tournament.
// Matches map 1:1 to our rows by the shared deterministic ExtID (no team
// name aliasing), and its `score.et` is already the cumulative after-120
// score — exactly our model.
const ofLiveURL = "https://raw.githubusercontent.com/openfootball/worldcup.json/master/2026/worldcup.json"

type ofScore struct {
	FT []int `json:"ft"`
	ET []int `json:"et"`
	P  []int `json:"p"`
}
type ofLiveMatch struct {
	Round  string       `json:"round"`
	Num    int          `json:"num"`
	Team1  string       `json:"team1"`
	Team2  string       `json:"team2"`
	Group  string       `json:"group"`
	Score  *ofScore     `json:"score"`
	Goals1 []ofLiveGoal `json:"goals1"`
	Goals2 []ofLiveGoal `json:"goals2"`
}

type ofLiveGoal struct {
	Name    string `json:"name"`
	Minute  string `json:"minute"`
	Penalty bool   `json:"penalty"`
	OwnGoal bool   `json:"owngoal"`
}

func pi(v int) *int { return &v }

// openfootballSync pulls openfootball's live JSON and applies any results.
// Idempotent: a record is only saved when something actually changed.
func openfootballSync(ctx context.Context, app core.App) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ofLiveURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wm-tips/1.0")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("openfootball fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("openfootball: status %d", resp.StatusCode)
	}
	var doc struct {
		Matches []ofLiveMatch `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}

	byExt := map[string]*core.Record{}
	recs, err := app.FindRecordsByFilter("matches", "id != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range recs {
		byExt[r.GetString("extId")] = r
	}

	updated := 0
	eventsUpdated := 0
	for _, m := range doc.Matches {
		if m.Score == nil || len(m.Score.FT) != 2 {
			continue // not played yet
		}
		rec := byExt[seed.ExtID(m.Round, m.Num, m.Group, m.Team1, m.Team2)]
		if rec == nil {
			continue
		}
		eventsUpdated += syncOpenfootballGoalEvents(app, rec, m)
		ftH, ftA := m.Score.FT[0], m.Score.FT[1]
		var etH, etA, penH, penA *int
		if len(m.Score.ET) == 2 { // cumulative after-120
			etH, etA = pi(m.Score.ET[0]), pi(m.Score.ET[1])
		}
		if len(m.Score.P) == 2 {
			penH, penA = pi(m.Score.P[0]), pi(m.Score.P[1])
		}
		// Skip if nothing changed (avoids needless recompute storms).
		if rec.GetString("status") == "finished" &&
			rec.GetInt("ftHome") == ftH && rec.GetInt("ftAway") == ftA &&
			rec.GetInt("penHome") == ip(penH) && rec.GetInt("penAway") == ip(penA) &&
			rec.GetInt("etHome") == ip(etH) && rec.GetInt("etAway") == ip(etA) {
			continue
		}
		applyResult(rec, "finished", pi(ftH), pi(ftA), etH, etA, penH, penA)
		if app.Save(rec) == nil {
			updated++
		}
	}
	if err := ResolveBracket(app); err != nil {
		return err
	}
	if eventsUpdated > 0 {
		fmt.Printf("[sync] openfootball goal events updated=%d\n", eventsUpdated)
	}
	return nil
}

func syncOpenfootballGoalEvents(app core.App, match *core.Record, m ofLiveMatch) int {
	eventsCol, err := app.FindCollectionByNameOrId(matchEventsCollection)
	if err != nil {
		return 0
	}
	updated := 0
	for _, item := range openfootballGoalEvents(match.Id, m) {
		existing, _ := app.FindFirstRecordByFilter(matchEventsCollection,
			"match = {:m} && providerKey = {:k}",
			map[string]any{"m": match.Id, "k": item.providerKey})
		rec := existing
		if rec == nil {
			rec = core.NewRecord(eventsCol)
			rec.Set("match", match.Id)
			rec.Set("providerKey", item.providerKey)
		}
		rec.Set("elapsed", item.elapsed)
		rec.Set("extra", item.extra)
		rec.Set("type", "Goal")
		rec.Set("detail", item.detail)
		rec.Set("player", item.player)
		rec.Set("assist", "")
		rec.Set("team", item.team)
		rec.Set("comments", item.comments)
		if err := app.Save(rec); err == nil {
			updated++
		}
	}
	return updated
}

type openfootballGoalEvent struct {
	providerKey string
	elapsed     int
	extra       int
	detail      string
	player      string
	team        string
	comments    string
}

func openfootballGoalEvents(matchID string, m ofLiveMatch) []openfootballGoalEvent {
	events := make([]openfootballGoalEvent, 0, len(m.Goals1)+len(m.Goals2))
	appendGoals := func(scoringTeam string, goals []ofLiveGoal, side string) {
		for i, goal := range goals {
			player := strings.TrimSpace(goal.Name)
			if player == "" {
				continue
			}
			elapsed, extra := parseOpenfootballMinute(goal.Minute)
			detail := "Normal Goal"
			switch {
			case goal.OwnGoal:
				detail = "Own Goal"
			case goal.Penalty:
				detail = "Penalty"
			}
			events = append(events, openfootballGoalEvent{
				providerKey: openfootballGoalProviderKey(matchID, side, i, goal),
				elapsed:     elapsed,
				extra:       extra,
				detail:      detail,
				player:      player,
				team:        strings.TrimSpace(scoringTeam),
				comments:    "Goal summary from openfootball; assists/cards/substitutions unavailable.",
			})
		}
	}
	appendGoals(m.Team1, m.Goals1, "h")
	appendGoals(m.Team2, m.Goals2, "a")
	return events
}

func openfootballGoalProviderKey(matchID, side string, idx int, goal ofLiveGoal) string {
	parts := []string{matchID, side, strconv.Itoa(idx), goal.Minute, goal.Name, strconv.FormatBool(goal.Penalty), strconv.FormatBool(goal.OwnGoal)}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "ofgoal:" + hex.EncodeToString(sum[:])
}

func parseOpenfootballMinute(raw string) (int, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0
	}
	parts := strings.SplitN(raw, "+", 2)
	elapsed, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || elapsed < 0 {
		return 0, 0
	}
	if len(parts) == 1 {
		return elapsed, 0
	}
	extra, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || extra < 0 {
		return elapsed, 0
	}
	return elapsed, extra
}
