package assets

import (
	"embed"
	"fmt"
	"path/filepath"
)

//go:embed configs/*
var configurations embed.FS

func GetConfiguration(name string) ([]byte, error) {
	path := filepath.Join("configs", name)
	switch name {
	case "cluster", "cluster.yaml":
		return configurations.ReadFile(path)
	case "site", "site.yaml":
		return configurations.ReadFile(path)
	default:
		return nil, fmt.Errorf("configuration not found: %s", name)
	}
}
