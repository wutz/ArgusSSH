package main

import (
	"flag"
	"log"

	"github.com/wutz/argusssh/internal/config"
	"github.com/wutz/argusssh/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	log.Printf("Starting ArgusSSH server...")
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
