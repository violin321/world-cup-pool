package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// 0029 adds Simplified Chinese to the persisted user language choices.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if f, ok := users.Fields.GetByName("language").(*core.SelectField); ok && !hasSelectValue(f.Values, "zh-CN") {
			f.Values = append(f.Values, "zh-CN")
		}
		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		if f, ok := users.Fields.GetByName("language").(*core.SelectField); ok {
			values := f.Values[:0]
			for _, value := range f.Values {
				if value != "zh-CN" {
					values = append(values, value)
				}
			}
			f.Values = values
		}
		return app.Save(users)
	})
}

func hasSelectValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
