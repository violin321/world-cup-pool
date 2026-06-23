package topscorer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/oyvhov/world-cup-pool/internal/football"
)

const (
	openfootballTopScorersURL = "https://raw.githubusercontent.com/openfootball/worldcup.json/master/2026/worldcup.json"
	openfootballSourceLabel   = "openfootball"
)

type openfootballTopScorerSource struct {
	url  string
	http *http.Client
}

type openfootballGoal struct {
	Name    string `json:"name"`
	OwnGoal bool   `json:"owngoal"`
}

type openfootballMatch struct {
	Team1  string             `json:"team1"`
	Team2  string             `json:"team2"`
	Goals1 []openfootballGoal `json:"goals1"`
	Goals2 []openfootballGoal `json:"goals2"`
}

func newOpenfootballTopScorerSource() *openfootballTopScorerSource {
	return &openfootballTopScorerSource{
		url:  openfootballTopScorersURL,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *openfootballTopScorerSource) TopScorers(ctx context.Context) ([]football.TopScorer, error) {
	endpoint := s.url
	if endpoint == "" {
		endpoint = openfootballTopScorersURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wm-tips/1.0")
	client := s.http
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openfootball top scorers fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openfootball top scorers: status %d", resp.StatusCode)
	}
	var doc struct {
		Matches []openfootballMatch `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return aggregateOpenfootballTopScorers(doc.Matches), nil
}

func (s *openfootballTopScorerSource) SearchPlayers(context.Context, string) ([]football.PlayerSearchResult, error) {
	return nil, nil
}

func aggregateOpenfootballTopScorers(matches []openfootballMatch) []football.TopScorer {
	type aggregate struct {
		name  string
		team  string
		goals int
	}
	byKey := map[string]*aggregate{}
	add := func(team string, goals []openfootballGoal) {
		team = strings.TrimSpace(team)
		for _, goal := range goals {
			name := strings.TrimSpace(goal.Name)
			if name == "" || goal.OwnGoal {
				continue
			}
			key := canonPlayer(name) + ":" + canonTeam(team)
			row := byKey[key]
			if row == nil {
				row = &aggregate{name: name, team: team}
				byKey[key] = row
			}
			row.goals++
		}
	}
	for _, match := range matches {
		add(match.Team1, match.Goals1)
		add(match.Team2, match.Goals2)
	}
	players := make([]football.TopScorer, 0, len(byKey))
	for _, row := range byKey {
		if row.goals == 0 {
			continue
		}
		players = append(players, football.TopScorer{Name: row.name, TeamName: row.team, Goals: row.goals})
	}
	sort.SliceStable(players, func(i, j int) bool {
		if players[i].Goals != players[j].Goals {
			return players[i].Goals > players[j].Goals
		}
		if players[i].TeamName != players[j].TeamName {
			return players[i].TeamName < players[j].TeamName
		}
		return players[i].Name < players[j].Name
	})
	for i := range players {
		if i > 0 && players[i].Goals == players[i-1].Goals {
			players[i].Rank = players[i-1].Rank
		} else {
			players[i].Rank = i + 1
		}
	}
	return players
}
