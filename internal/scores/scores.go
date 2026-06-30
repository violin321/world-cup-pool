package scores

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/oyvhov/world-cup-pool/internal/football"
)

const defaultBaseURL = "https://wc.violinai.qzz.io/scores-api"

type client struct {
	base string
	http *http.Client
}

type listResponse struct {
	Success bool         `json:"success"`
	Data    []scoreMatch `json:"data"`
}

type detailResponse struct {
	Success bool       `json:"success"`
	Data    scoreMatch `json:"data"`
}

type scoreMatch struct {
	ID          string `json:"id"`
	HomeTeam    team   `json:"homeTeam"`
	AwayTeam    team   `json:"awayTeam"`
	BeijingTime struct {
		Timestamp int64  `json:"timestamp"`
		Full      string `json:"full"`
	} `json:"beijingTime"`
	Status      string          `json:"status"`
	TimeElapsed string          `json:"timeElapsed"`
	PeriodCn    string          `json:"periodCn"`
	Finished    bool            `json:"finished"`
	HomeScore   int             `json:"homeScore"`
	AwayScore   int             `json:"awayScore"`
	LiveScore   scorePair       `json:"liveScore"`
	Events      []scoreEvent    `json:"events"`
	Stats       json.RawMessage `json:"stats"`
	DataSource  string          `json:"dataSource"`
}

type team struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NameZh string `json:"nameZh"`
	Score  int    `json:"score"`
}

type scorePair struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

type scoreEvent struct {
	Minute    int    `json:"minute"`
	Type      string `json:"type"`
	EventCode int    `json:"event_code"`
	EventCn   string `json:"event_cn"`
	Info      string `json:"info"`
	Team      string `json:"team"`
	Player    string `json:"player"`
	Assist    string `json:"assist"`
}

type scoreEventJSON struct {
	ID          string `json:"id"`
	Match       string `json:"match"`
	ProviderKey string `json:"providerKey"`
	Created     string `json:"created"`
	Elapsed     int    `json:"elapsed"`
	Extra       int    `json:"extra"`
	Type        string `json:"type"`
	Detail      string `json:"detail"`
	Player      string `json:"player"`
	Assist      string `json:"assist"`
	Team        string `json:"team"`
	TeamId      string `json:"teamId"`
	Comments    string `json:"comments"`
}

type goalJSON struct {
	Minute string `json:"minute"`
	Player string `json:"player"`
	Assist string `json:"assist,omitempty"`
	Team   string `json:"team"`
	TeamId string `json:"teamId"`
	Detail string `json:"detail,omitempty"`
}

// Register adds a scores-api-backed match detail endpoint keyed by the local
// PocketBase match id. Mapping is dynamic: first exact team pair, then kickoff
// proximity, with list order as the final tie-breaker.
func Register(app core.App, se *core.ServeEvent) {
	se.Router.GET("/api/matches/{id}/scores", func(e *core.RequestEvent) error {
		matchID := e.Request.PathValue("id")
		if matchID == "" {
			return e.JSON(http.StatusBadRequest, map[string]string{"error": "missing match id"})
		}
		match, err := app.FindRecordById("matches", matchID)
		if err != nil {
			return apis.NewNotFoundError("match not found", err)
		}
		ctx, cancel := context.WithTimeout(e.Request.Context(), 12*time.Second)
		defer cancel()
		detail, matchedBy, err := newClient().MatchForLocal(ctx, app, match)
		if err != nil {
			return e.JSON(http.StatusOK, emptyPayload(matchID, err.Error()))
		}
		payload := normalizeMatch(app, match, detail, matchedBy)
		return e.JSON(http.StatusOK, payload)
	}).Bind(apis.RequireAuth())
}

func newClient() *client {
	base := strings.TrimRight(os.Getenv("WORLDCUP_SCORES_API_BASE"), "/")
	if base == "" {
		base = defaultBaseURL
	}
	return &client{base: base, http: &http.Client{Timeout: 12 * time.Second}}
}

func (c *client) MatchForLocal(ctx context.Context, app core.App, match *core.Record) (scoreMatch, string, error) {
	list, err := c.matches(ctx)
	if err != nil {
		return scoreMatch{}, "", err
	}
	if len(list) == 0 {
		return scoreMatch{}, "", fmt.Errorf("scores-api returned no matches")
	}
	candidate, matchedBy, ok := bestMatch(app, match, list)
	if !ok || candidate.ID == "" {
		return scoreMatch{}, "", fmt.Errorf("scores-api match mapping not found")
	}
	detail, err := c.match(ctx, candidate.ID)
	if err != nil {
		return scoreMatch{}, "", err
	}
	return detail, matchedBy, nil
}

func (c *client) matches(ctx context.Context) ([]scoreMatch, error) {
	var body listResponse
	if err := c.get(ctx, "/matches", &body); err != nil {
		return nil, err
	}
	return body.Data, nil
}

func (c *client) match(ctx context.Context, id string) (scoreMatch, error) {
	var body detailResponse
	if err := c.get(ctx, "/matches/"+id, &body); err != nil {
		return scoreMatch{}, err
	}
	return body.Data, nil
}

func (c *client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scores-api %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func bestMatch(app core.App, match *core.Record, candidates []scoreMatch) (scoreMatch, string, bool) {
	homeName, awayName := localTeamNames(app, match)
	homeCanon, awayCanon := canonTeam(homeName), canonTeam(awayName)
	kickoff := match.GetDateTime("kickoff").Time()
	if !kickoff.IsZero() {
		kickoff = kickoff.UTC()
	}

	type ranked struct {
		match     scoreMatch
		teamScore int
		timeDiff  time.Duration
		idx       int
	}
	rankedCandidates := make([]ranked, 0, len(candidates))
	for i, candidate := range candidates {
		teamScore := 0
		ch, ca := canonTeam(candidate.HomeTeam.Name), canonTeam(candidate.AwayTeam.Name)
		if homeCanon != "" && awayCanon != "" {
			switch {
			case ch == homeCanon && ca == awayCanon:
				teamScore = 3
			case ch == awayCanon && ca == homeCanon:
				teamScore = 2
			case ch == homeCanon || ca == awayCanon || ch == awayCanon || ca == homeCanon:
				teamScore = 1
			}
		}
		timeDiff := 365 * 24 * time.Hour
		if ts := candidate.BeijingTime.Timestamp; ts > 0 && !kickoff.IsZero() {
			candidateTime := time.UnixMilli(ts).UTC()
			timeDiff = absDuration(candidateTime.Sub(kickoff))
		}
		rankedCandidates = append(rankedCandidates, ranked{match: candidate, teamScore: teamScore, timeDiff: timeDiff, idx: i})
	}
	sort.SliceStable(rankedCandidates, func(i, j int) bool {
		left, right := rankedCandidates[i], rankedCandidates[j]
		if left.teamScore != right.teamScore {
			return left.teamScore > right.teamScore
		}
		if left.timeDiff != right.timeDiff {
			return left.timeDiff < right.timeDiff
		}
		return left.idx < right.idx
	})
	best := rankedCandidates[0]
	if best.teamScore >= 2 {
		return best.match, "teams", true
	}
	if best.teamScore >= 1 && best.timeDiff <= 12*time.Hour {
		return best.match, "teams+kickoff", true
	}
	if best.teamScore == 0 && best.timeDiff <= 2*time.Hour {
		return best.match, "kickoff", true
	}
	return scoreMatch{}, "", false
}

func localTeamNames(app core.App, match *core.Record) (string, string) {
	lookup := func(field string) string {
		id := match.GetString(field)
		if id == "" {
			return ""
		}
		team, err := app.FindRecordById("teams", id)
		if err != nil {
			return ""
		}
		return team.GetString("name")
	}
	return lookup("homeTeam"), lookup("awayTeam")
}

func normalizeMatch(app core.App, match *core.Record, detail scoreMatch, matchedBy string) map[string]any {
	events := normalizeEvents(match, detail)
	goals := make([]goalJSON, 0)
	for _, event := range events {
		if event.Type != "Goal" {
			continue
		}
		goals = append(goals, goalJSON{
			Minute: eventMinute(event.Elapsed, event.Extra),
			Player: event.Player,
			Assist: event.Assist,
			Team:   event.Team,
			TeamId: event.TeamId,
			Detail: event.Detail,
		})
	}
	return map[string]any{
		"matchId":       match.Id,
		"scoresMatchId": detail.ID,
		"matchedBy":     matchedBy,
		"source":        "scores",
		"dataSource":    detail.DataSource,
		"status":        detail.Status,
		"period":        firstNonEmpty(detail.PeriodCn, detail.TimeElapsed),
		"score": map[string]int{
			"home": firstNonZero(detail.LiveScore.Home, detail.HomeScore, detail.HomeTeam.Score),
			"away": firstNonZero(detail.LiveScore.Away, detail.AwayScore, detail.AwayTeam.Score),
		},
		"teams": map[string]any{
			"home": map[string]string{"id": match.GetString("homeTeam"), "name": detail.HomeTeam.Name, "nameZh": detail.HomeTeam.NameZh},
			"away": map[string]string{"id": match.GetString("awayTeam"), "name": detail.AwayTeam.Name, "nameZh": detail.AwayTeam.NameZh},
		},
		"goals":  goals,
		"events": events,
		"stats":  parseStats(detail.Stats),
	}
}

func normalizeEvents(match *core.Record, detail scoreMatch) []scoreEventJSON {
	out := make([]scoreEventJSON, 0, len(detail.Events))
	for i, event := range detail.Events {
		typ, detailText := normalizeEventType(event)
		teamID, teamName := eventTeam(match, detail, event.Team)
		key := scoresEventKey(detail.ID, i, event)
		out = append(out, scoreEventJSON{
			ID:          key,
			Match:       match.Id,
			ProviderKey: key,
			Elapsed:     event.Minute,
			Type:        typ,
			Detail:      detailText,
			Player:      strings.TrimSpace(event.Player),
			Assist:      strings.TrimSpace(event.Assist),
			Team:        teamName,
			TeamId:      teamID,
			Comments:    strings.TrimSpace(firstNonEmpty(event.Info, event.EventCn)),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Elapsed < out[j].Elapsed
	})
	return out
}

func normalizeEventType(event scoreEvent) (string, string) {
	typeText := strings.ToLower(strings.TrimSpace(event.Type))
	switch {
	case strings.Contains(typeText, "goal"):
		return "Goal", firstNonEmpty(event.EventCn, "Goal")
	case strings.Contains(typeText, "red_card"):
		return "Card", "Red Card"
	case strings.Contains(typeText, "yellow_card"):
		return "Card", "Yellow Card"
	case strings.Contains(typeText, "sub"):
		return "subst", firstNonEmpty(event.EventCn, "Substitution")
	case strings.Contains(typeText, "var"):
		return "Var", firstNonEmpty(event.EventCn, "VAR")
	default:
		return firstNonEmpty(event.Type, "Event"), firstNonEmpty(event.EventCn, event.Info)
	}
}

func eventTeam(match *core.Record, detail scoreMatch, side string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "home", "1":
		return match.GetString("homeTeam"), detail.HomeTeam.Name
	case "away", "2":
		return match.GetString("awayTeam"), detail.AwayTeam.Name
	default:
		return "", ""
	}
}

func scoresEventKey(matchID string, idx int, event scoreEvent) string {
	parts := []string{matchID, strconv.Itoa(idx), strconv.Itoa(event.Minute), event.Type, event.Team, event.Player, event.Assist, event.Info}
	sum := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "scores:" + hex.EncodeToString(sum[:])
}

func parseStats(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func emptyPayload(matchID, reason string) map[string]any {
	return map[string]any{
		"matchId": matchID,
		"source":  "scores",
		"error":   reason,
		"goals":   []any{},
		"events":  []any{},
		"stats":   map[string]any{},
	}
}

func canonTeam(name string) string {
	switch football.NormalizeName(name) {
	case "korearepublic":
		return football.NormalizeName("South Korea")
	case "czechia":
		return football.NormalizeName("Czech Republic")
	case "usa":
		return football.NormalizeName("United States")
	case "bosniaandherzegovina":
		return football.NormalizeName("Bosnia & Herzegovina")
	case "ivorycoast":
		return football.NormalizeName("Côte d'Ivoire")
	case "congodr", "democraticrepublicofcongo":
		return football.NormalizeName("DR Congo")
	case "capeverdeislands":
		return football.NormalizeName("Cape Verde")
	case "turkiye":
		return football.NormalizeName("Turkey")
	default:
		return football.NormalizeName(name)
	}
}

func absDuration(v time.Duration) time.Duration {
	if v < 0 {
		return -v
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func eventMinute(elapsed, extra int) string {
	if elapsed <= 0 {
		return ""
	}
	if extra > 0 {
		return fmt.Sprintf("%d+%d'", elapsed, extra)
	}
	return fmt.Sprintf("%d'", elapsed)
}
