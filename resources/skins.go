package resources

import (
	_ "embed"
	"encoding/json"
)

//go:embed skins.json
var skinData []byte

type Skin struct {
	Label  string            `json:"label"`
	Radius float32           `json:"radius"`
	Colors map[string]string `json:"colors"`
}

var skins = func() map[string]Skin {
	var result map[string]Skin
	if err := json.Unmarshal(skinData, &result); err != nil {
		panic(err)
	}
	return result
}()

func LookupSkin(id string) (Skin, bool) {
	skin, ok := skins[id]
	return skin, ok
}
