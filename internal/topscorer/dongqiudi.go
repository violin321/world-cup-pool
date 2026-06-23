package topscorer

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oyvhov/world-cup-pool/internal/football"
)

const (
	dongqiudiTopScorersURL = "https://m.dongqiudi.com/stat/9/rankingGoal"
	dongqiudiSourceLabel   = "懂球帝"
	dongqiudiMinFetchEvery = 45 * time.Minute
)

type dongqiudiTopScorerSource struct {
	url        string
	http       *http.Client
	minFetch   time.Duration
	now        func() time.Time
	mu         sync.Mutex
	lastFetch  time.Time
	lastResult []football.TopScorer
	lastErr    error
}

func newDongqiudiTopScorerSource() *dongqiudiTopScorerSource {
	return &dongqiudiTopScorerSource{
		url:      dongqiudiTopScorersURL,
		http:     &http.Client{Timeout: 12 * time.Second},
		minFetch: dongqiudiMinFetchEvery,
		now:      time.Now,
	}
}

func (s *dongqiudiTopScorerSource) TopScorers(ctx context.Context) ([]football.TopScorer, error) {
	now := s.now()

	s.mu.Lock()
	if !s.lastFetch.IsZero() && now.Sub(s.lastFetch) < s.minFetch {
		cached, err := cloneTopScorers(s.lastResult), s.lastErr
		s.mu.Unlock()
		if len(cached) > 0 {
			return cached, nil
		}
		if err != nil {
			return nil, err
		}
		return []football.TopScorer{}, nil
	}
	s.mu.Unlock()

	players, err := s.fetch(ctx)

	s.mu.Lock()
	s.lastFetch = now
	s.lastResult = cloneTopScorers(players)
	s.lastErr = err
	s.mu.Unlock()

	return players, err
}

func (s *dongqiudiTopScorerSource) fetch(ctx context.Context) ([]football.TopScorer, error) {
	endpoint := s.url
	if endpoint == "" {
		endpoint = dongqiudiTopScorersURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 WorldCupPoolBot/1.0 (+https://github.com/violin321/world-cup-pool)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	client := s.http
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dongqiudi top scorers: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	players, err := parseDongqiudiTopScorers(string(body))
	if err != nil {
		return nil, err
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("dongqiudi top scorers: no rows parsed")
	}
	return players, nil
}

func (s *dongqiudiTopScorerSource) SearchPlayers(context.Context, string) ([]football.PlayerSearchResult, error) {
	return nil, nil
}

var dongqiudiGoalRowRE = regexp.MustCompile(`(?is)<tr>\s*<td[^>]*class="[^"]*rank[^"]*"[^>]*>\s*([0-9]+)\s*</td>\s*<td[^>]*class="[^"]*person[^"]*"[^>]*>.*?<span[^>]*class="[^"]*name[^"]*"[^>]*>\s*([^<]+?)\s*</span>\s*</td>\s*<td[^>]*class="[^"]*team[^"]*"[^>]*>\s*([^<]+?)\s*</td>\s*<td[^>]*class="[^"]*font-family-number[^"]*"[^>]*>\s*([0-9]+)(?:\s*\((\d+)\))?\s*</td>`)

func parseDongqiudiTopScorers(page string) ([]football.TopScorer, error) {
	rows := dongqiudiGoalRowRE.FindAllStringSubmatch(page, -1)
	players := make([]football.TopScorer, 0, len(rows))
	for _, row := range rows {
		rank := atoi(row[1])
		name := cleanDongqiudiText(row[2])
		team := cleanDongqiudiText(row[3])
		goals := atoi(row[4])
		if rank <= 0 || name == "" || team == "" || goals <= 0 {
			continue
		}
		players = append(players, football.TopScorer{
			ProviderID: 0,
			Name:       name,
			TeamName:   team,
			Goals:      goals,
			Assists:    0,
			Rank:       rank,
		})
	}
	return players, nil
}

func cleanDongqiudiText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.Join(strings.Fields(value), " ")
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(value))
	return n
}

func cloneTopScorers(players []football.TopScorer) []football.TopScorer {
	if len(players) == 0 {
		return nil
	}
	cloned := make([]football.TopScorer, len(players))
	copy(cloned, players)
	return cloned
}
