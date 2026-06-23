package admin

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/oyvhov/world-cup-pool/internal/scoring"
	wmsync "github.com/oyvhov/world-cup-pool/internal/sync"
)

func bad(e *core.RequestEvent, code int, msg string) error {
	return e.JSON(code, map[string]string{"error": msg})
}

func dateString(rec *core.Record, field string) string {
	dt := rec.GetDateTime(field)
	if dt.IsZero() {
		return ""
	}
	return dt.Time().Format(time.RFC3339Nano)
}

func teamMap(app core.App) map[string]string {
	out := map[string]string{}
	teams, err := app.FindRecordsByFilter("teams", "id != ''", "name", 0, 0)
	if err != nil {
		return out
	}
	for _, team := range teams {
		out[team.Id] = team.GetString("name")
	}
	return out
}

func userMap(app core.App) map[string]map[string]string {
	out := map[string]map[string]string{}
	users, err := app.FindRecordsByFilter("users", "id != ''", "name", 0, 0)
	if err != nil {
		return out
	}
	for _, user := range users {
		name := strings.TrimSpace(user.GetString("name"))
		if name == "" {
			name = user.GetString("email")
		}
		out[user.Id] = map[string]string{"name": name, "email": user.GetString("email")}
	}
	return out
}

func matchMap(app core.App, teams map[string]string) map[string]map[string]any {
	out := map[string]map[string]any{}
	matches, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	if err != nil {
		return out
	}
	for _, m := range matches {
		out[m.Id] = matchDTO(m, teams)
	}
	return out
}

func matchDTO(m *core.Record, teams map[string]string) map[string]any {
	homeID := m.GetString("homeTeam")
	awayID := m.GetString("awayTeam")
	homeName := teams[homeID]
	awayName := teams[awayID]
	if homeName == "" {
		homeName = m.GetString("homeLabel")
	}
	if awayName == "" {
		awayName = m.GetString("awayLabel")
	}
	return map[string]any{
		"id":          m.Id,
		"extId":       m.GetString("extId"),
		"stage":       m.GetString("stage"),
		"num":         m.GetInt("num"),
		"groupLetter": m.GetString("groupLetter"),
		"roundLabel":  m.GetString("roundLabel"),
		"kickoff":     dateString(m, "kickoff"),
		"status":      m.GetString("status"),
		"homeTeamId":  homeID,
		"awayTeamId":  awayID,
		"homeTeam":    homeName,
		"awayTeam":    awayName,
		"homeLabel":   m.GetString("homeLabel"),
		"awayLabel":   m.GetString("awayLabel"),
		"ftHome":      m.GetInt("ftHome"),
		"ftAway":      m.GetInt("ftAway"),
		"etHome":      m.GetInt("etHome"),
		"etAway":      m.GetInt("etAway"),
		"penHome":     m.GetInt("penHome"),
		"penAway":     m.GetInt("penAway"),
		"finalizedAt": dateString(m, "finalizedAt"),
	}
}

func appMetaRecord(app core.App, key string) (*core.Record, error) {
	rec, err := app.FindFirstRecordByFilter("app_meta", "key = {:key}", map[string]any{"key": key})
	if err == nil {
		return rec, nil
	}
	col, err := app.FindCollectionByNameOrId("app_meta")
	if err != nil {
		return nil, err
	}
	rec = core.NewRecord(col)
	rec.Set("key", key)
	return rec, nil
}

func defaultLanguage(app core.App) string {
	rec, err := app.FindFirstRecordByFilter("app_meta", "key = {:key}", map[string]any{"key": "default_language"})
	if err != nil {
		return "en"
	}
	var value string
	if err := rec.UnmarshalJSONField("value", &value); err != nil || value == "" {
		value = strings.Trim(strings.TrimSpace(fmt.Sprint(rec.Get("value"))), "\"")
	}
	if isLanguage(value) {
		return value
	}
	return "en"
}

func isLanguage(v string) bool {
	switch v {
	case "en", "zh-CN", "nb", "nn":
		return true
	default:
		return false
	}
}

func randomPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "Wcp-" + base64.RawURLEncoding.EncodeToString(b), nil
}

func envSet(name string) bool { return strings.TrimSpace(os.Getenv(name)) != "" }

func Register(app core.App, se *core.ServeEvent) {
	se.Router.GET("/api/app-settings", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]any{"defaultLanguage": defaultLanguage(app)})
	})

	admin := se.Router.Group("/api/admin")
	admin.Bind(apis.RequireSuperuserAuth())

	admin.GET("/me", func(e *core.RequestEvent) error {
		email := ""
		if e.Auth != nil {
			email = e.Auth.GetString("email")
		}
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "superuser": true, "email": email})
	})

	admin.GET("/summary", func(e *core.RequestEvent) error {
		users, _ := app.CountRecords("users")
		leagues, _ := app.CountRecords("leagues")
		tips, _ := app.CountRecords("tips")
		matches, _ := app.CountRecords("matches")
		finished, _ := app.CountRecords("matches", dbx.HashExp{"status": "finished"})
		live, _ := app.CountRecords("matches", dbx.HashExp{"status": "live"})
		scheduled, _ := app.CountRecords("matches", dbx.HashExp{"status": "scheduled"})
		return e.JSON(http.StatusOK, map[string]any{
			"users": users, "leagues": leagues, "tips": tips,
			"matches": map[string]int64{"total": matches, "finished": finished, "live": live, "scheduled": scheduled},
		})
	})

	admin.GET("/matches", func(e *core.RequestEvent) error {
		teams := teamMap(app)
		matches, err := app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
		if err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		out := make([]map[string]any, 0, len(matches))
		for _, m := range matches {
			out = append(out, matchDTO(m, teams))
		}
		return e.JSON(http.StatusOK, map[string]any{"items": out})
	})

	admin.POST("/matches/{id}/result", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		rec, err := app.FindRecordById("matches", id)
		if err != nil {
			return bad(e, http.StatusNotFound, "match not found")
		}
		var body struct {
			FTHome, FTAway   *int   `json:"ftHome"`
			ETHome, ETAway   *int   `json:"etHome"`
			PenHome, PenAway *int   `json:"penHome"`
			Status           string `json:"status"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, "invalid body")
		}
		old := fmt.Sprintf("%s %d-%d", rec.GetString("status"), rec.GetInt("ftHome"), rec.GetInt("ftAway"))
		wmsync.ApplyResult(rec, body.Status, body.FTHome, body.FTAway, body.ETHome, body.ETAway, body.PenHome, body.PenAway)
		if err := app.Save(rec); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		if err := wmsync.ResolveBracket(app); err != nil {
			log.Printf("[admin] resolve after result override match=%s: %v", rec.Id, err)
		}
		log.Printf("[admin] result override by superuser=%s match=%s old=%s new=%s %d-%d", e.Auth.Id, rec.Id, old, rec.GetString("status"), rec.GetInt("ftHome"), rec.GetInt("ftAway"))
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "match": matchDTO(rec, teamMap(app))})
	})

	admin.POST("/recompute", func(e *core.RequestEvent) error {
		if err := scoring.Recompute(app); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		log.Printf("[admin] recompute by superuser=%s", e.Auth.Id)
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	})

	admin.GET("/tips", func(e *core.RequestEvent) error {
		q := e.Request.URL.Query()
		userQ := strings.ToLower(strings.TrimSpace(q.Get("user")))
		matchQ := strings.ToLower(strings.TrimSpace(q.Get("match")))
		users := userMap(app)
		teams := teamMap(app)
		matches := matchMap(app, teams)
		recs, err := app.FindRecordsByFilter("tips", "id != ''", "-updated", 200, 0)
		if err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		out := []map[string]any{}
		for _, tip := range recs {
			u := users[tip.GetString("user")]
			m := matches[tip.GetString("match")]
			matchText := strings.ToLower(fmt.Sprintf("%v %v %v", m["homeTeam"], m["awayTeam"], m["kickoff"]))
			userText := strings.ToLower(u["name"] + " " + u["email"])
			if userQ != "" && !strings.Contains(userText, userQ) && tip.GetString("user") != userQ {
				continue
			}
			if matchQ != "" && !strings.Contains(matchText, matchQ) && tip.GetString("match") != matchQ {
				continue
			}
			out = append(out, map[string]any{
				"id": tip.Id, "userId": tip.GetString("user"), "userName": u["name"], "userEmail": u["email"],
				"matchId": tip.GetString("match"), "match": m,
				"ftHome": tip.GetInt("ftHome"), "ftAway": tip.GetInt("ftAway"), "etHome": tip.GetInt("etHome"), "etAway": tip.GetInt("etAway"),
				"created": dateString(tip, "created"), "updated": dateString(tip, "updated"),
			})
		}
		return e.JSON(http.StatusOK, map[string]any{"items": out})
	})

	admin.DELETE("/tips/{id}", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		tip, err := app.FindRecordById("tips", id)
		if err != nil {
			return bad(e, http.StatusNotFound, "tip not found")
		}
		userID, matchID := tip.GetString("user"), tip.GetString("match")
		if err := app.Delete(tip); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		if err := scoring.Recompute(app); err != nil {
			log.Printf("[admin] recompute after tip delete: %v", err)
		}
		log.Printf("[admin] tip deleted by superuser=%s tip=%s user=%s match=%s", e.Auth.Id, id, userID, matchID)
		return e.JSON(http.StatusOK, map[string]any{"ok": true})
	})

	admin.GET("/users", func(e *core.RequestEvent) error {
		users, err := app.FindRecordsByFilter("users", "id != ''", "name", 0, 0)
		if err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		out := []map[string]any{}
		for _, user := range users {
			tipCount, _ := app.CountRecords("tips", dbx.HashExp{"user": user.Id})
			leagueCount, _ := app.CountRecords("league_members", dbx.HashExp{"user": user.Id})
			ownedCount, _ := app.CountRecords("leagues", dbx.HashExp{"owner": user.Id})
			messageCount, _ := app.CountRecords("league_messages", dbx.HashExp{"user": user.Id})
			name := strings.TrimSpace(user.GetString("name"))
			if name == "" {
				name = user.GetString("email")
			}
			out = append(out, map[string]any{"id": user.Id, "name": name, "email": user.GetString("email"), "tips": tipCount, "leagues": leagueCount, "ownedLeagues": ownedCount, "messages": messageCount, "created": dateString(user, "created")})
		}
		return e.JSON(http.StatusOK, map[string]any{"items": out})
	})

	admin.POST("/users/{id}/reset-password", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		user, err := app.FindRecordById("users", id)
		if err != nil {
			return bad(e, http.StatusNotFound, "user not found")
		}
		var body struct {
			Password string `json:"password"`
			Generate bool   `json:"generate"`
			Confirm  string `json:"confirm"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, "invalid body")
		}
		if body.Confirm != "RESET" {
			return bad(e, http.StatusBadRequest, "confirmation required")
		}
		temp := strings.TrimSpace(body.Password)
		if body.Generate || temp == "" {
			temp, err = randomPassword()
			if err != nil {
				return bad(e, http.StatusInternalServerError, err.Error())
			}
		}
		if len(temp) < 10 {
			return bad(e, http.StatusBadRequest, "password must be at least 10 characters")
		}
		user.SetPassword(temp)
		if err := app.Save(user); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		log.Printf("[admin] user password reset by superuser=%s user=%s email=%s", e.Auth.Id, user.Id, user.GetString("email"))
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "temporaryPassword": temp})
	})

	admin.DELETE("/users/{id}", func(e *core.RequestEvent) error {
		id := e.Request.PathValue("id")
		user, err := app.FindRecordById("users", id)
		if err != nil {
			return bad(e, http.StatusNotFound, "user not found")
		}
		var body struct {
			Confirm string `json:"confirm"`
		}
		_ = e.BindBody(&body)
		if body.Confirm != "DELETE" {
			return bad(e, http.StatusBadRequest, "confirmation required")
		}
		tipCount, _ := app.CountRecords("tips", dbx.HashExp{"user": user.Id})
		leagueCount, _ := app.CountRecords("league_members", dbx.HashExp{"user": user.Id})
		ownedCount, _ := app.CountRecords("leagues", dbx.HashExp{"owner": user.Id})
		messageCount, _ := app.CountRecords("league_messages", dbx.HashExp{"user": user.Id})
		if err := app.Delete(user); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		if err := scoring.Recompute(app); err != nil {
			log.Printf("[admin] recompute after user delete: %v", err)
		}
		log.Printf("[admin] user deleted by superuser=%s user=%s email=%s tips=%d memberships=%d ownedLeagues=%d messages=%d", e.Auth.Id, id, user.GetString("email"), tipCount, leagueCount, ownedCount, messageCount)
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "deleted": id, "impact": map[string]int64{"tips": tipCount, "leagueMemberships": leagueCount, "ownedLeagues": ownedCount, "messages": messageCount}})
	})

	admin.GET("/settings", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]any{
			"defaultLanguage": defaultLanguage(app),
			"languages": []map[string]string{
				{"code": "en", "label": "English"},
				{"code": "zh-CN", "label": "简体中文"},
				{"code": "nb", "label": "Bokmål"},
				{"code": "nn", "label": "Nynorsk"},
			},
			"mail": map[string]any{
				"applicationUrlConfigured":   envSet("PUBLIC_BASE_URL") || envSet("APP_URL") || envSet("PB_PUBLIC_URL"),
				"signupAlertEmailConfigured": envSet("SIGNUP_ALERT_EMAIL"),
				"notifyCronEnabled":          strings.EqualFold(strings.TrimSpace(os.Getenv("NOTIFY_CRON_ENABLED")), "true"),
				"vapidPublicKeyConfigured":   envSet("VAPID_PUBLIC_KEY"),
				"vapidPrivateKeyConfigured":  envSet("VAPID_PRIVATE_KEY"),
				"smtpEnvConfigured":          envSet("SMTP_HOST") || envSet("SMTP_USERNAME") || envSet("SMTP_USER"),
				"pocketBaseSettingsUrl":      "/_/#/settings/mail",
			},
		})
	})

	admin.POST("/settings/default-language", func(e *core.RequestEvent) error {
		var body struct {
			Language string `json:"language"`
		}
		if err := e.BindBody(&body); err != nil {
			return bad(e, http.StatusBadRequest, "invalid body")
		}
		if !isLanguage(body.Language) {
			return bad(e, http.StatusBadRequest, "unsupported language")
		}
		rec, err := appMetaRecord(app, "default_language")
		if err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		rec.Set("value", body.Language)
		if err := app.Save(rec); err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		log.Printf("[admin] default language changed by superuser=%s language=%s", e.Auth.Id, body.Language)
		return e.JSON(http.StatusOK, map[string]any{"ok": true, "defaultLanguage": body.Language})
	})

	admin.GET("/leagues", func(e *core.RequestEvent) error {
		leagues, err := app.FindRecordsByFilter("leagues", "id != ''", "name", 0, 0)
		if err != nil {
			return bad(e, http.StatusInternalServerError, err.Error())
		}
		users := userMap(app)
		out := []map[string]any{}
		for _, league := range leagues {
			members, _ := app.CountRecords("league_members", dbx.HashExp{"league": league.Id})
			owner := users[league.GetString("owner")]
			out = append(out, map[string]any{"id": league.Id, "name": league.GetString("name"), "inviteCode": league.GetString("inviteCode"), "ownerName": owner["name"], "members": members, "created": dateString(league, "created")})
		}
		return e.JSON(http.StatusOK, map[string]any{"items": out})
	})
}
