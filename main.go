package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ezenkico/deploy-commander/runner/interfaces"
	"github.com/ezenkico/deploy-commander/runner/models"
)

const configPath = "/run/config.json"

func loadConfiguration(path string) (models.Configuration, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return models.Configuration{}, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg models.Configuration
	if err := json.Unmarshal(b, &cfg); err != nil {
		return models.Configuration{}, fmt.Errorf("parse config json %q: %w", path, err)
	}

	return cfg, nil
}

func selectPlatform(platform string) (interfaces.Platform, error) {
	switch platform {
	// TODO: add cases later, e.g.
	// case "docker":
	//     return docker.New(...), nil
	// case "k8s":
	//     return k8s.New(...), nil
	default:
		return nil, fmt.Errorf("%q is not a valid platform", platform)
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := loadConfiguration(configPath)
	if err != nil {
		log.Fatal(err)
	}

	p, err := selectPlatform(cfg.Platform)
	if err != nil {
		log.Fatal(err)
	}

	if err := p.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
