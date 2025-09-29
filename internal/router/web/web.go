package web

import (
	"embed"
	"fmt"
)

//go:embed *
var web embed.FS

func GetWebAsset(name string) ([]byte, error) {
	switch name {
	case "styles.css":
		return web.ReadFile("assets/styles.css")
	default:
		return nil, fmt.Errorf("web asset not found: %s", name)
	}
}
