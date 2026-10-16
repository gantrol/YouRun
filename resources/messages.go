// Package resources is the common presentation catalog for all three hosts.
package resources

import (
	"embed"
	"encoding/json"
)

//go:embed locales/*.json
var catalogs embed.FS

func Messages(locale string) map[string]string {
	if locale != "zh" {
		locale = "en"
	}
	data, err := catalogs.ReadFile("locales/" + locale + ".json")
	if err != nil {
		panic(err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}
