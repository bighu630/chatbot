package main

import (
	"chatbot/internal/app"
	"chatbot/pkg/config"
	"flag"
	"fmt"
	"log"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	fmt.Println("hello telegram bot")
	if err := app.Start(cfg); err != nil {
		log.Fatalf("bot exited with error: %v", err)
	}
}
