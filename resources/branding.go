package resources

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed branding/yourun-rabbit-paths.json
var rabbitPathData []byte

//go:embed branding/yourun-rabbit.png
var RabbitPNG []byte

type rabbitArtwork struct {
	Silhouette string `json:"silhouette"`
	Comic      string `json:"comic"`
	Ears       string `json:"ears"`
	Pixel      string `json:"pixel"`
}

var rabbitPaths = func() rabbitArtwork {
	var paths rabbitArtwork
	if err := json.Unmarshal(rabbitPathData, &paths); err != nil {
		panic(err)
	}
	return paths
}()

// RabbitLogoSVG returns compact artwork without CSS for native SVG renderers.
func RabbitLogoSVG(skinID string) string {
	skin, ok := LookupSkin(skinID)
	if !ok {
		skinID = "comic"
		skin, _ = LookupSkin(skinID)
	}
	fill := skin.Colors["button-primary"]
	if fill == "" {
		fill = "#80566a"
	}
	const eye = "M95.7 64a4.3 4.3 0 1 0 8.6 0a4.3 4.3 0 1 0 -8.6 0Z"
	path := rabbitPaths.Silhouette + eye
	switch skinID {
	case "comic":
		ink := skin.Colors["foreground"]
		return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><path d="%s" fill="%s" stroke="%s" stroke-width="4.8" stroke-linecap="round" stroke-linejoin="round"/><circle cx="100" cy="64" r="4.3" fill="%s"/></svg>`, rabbitPaths.Comic, fill, ink, ink)
	case "graphic":
		path = rabbitPaths.Silhouette + rabbitPaths.Ears + eye
	case "pixel":
		path = rabbitPaths.Pixel
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><path d="%s" fill="%s" fill-rule="evenodd"/></svg>`, path, fill)
}
