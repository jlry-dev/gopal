package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/jlry-dev/gopal/bot"
	"github.com/joho/godotenv"
)

func main() {
	if env := os.Getenv("APP_ENV"); env != "production" {
		err := godotenv.Load()
		if err != nil {
			log.Fatalf("failed to load env: %v", err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Setenv("WORKING_DIR", wd); err != nil {
		log.Fatalf("failed to set working directory env variable: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	bot := bot.NewBot(logger)

	bot.Run()
}
