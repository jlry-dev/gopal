package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/jlry-dev/gopal/bot/gopal"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
			log.Fatalf("Critical error reading .env configuration: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Setenv("WORKING_DIR", wd); err != nil {
		log.Fatalf("failed to set working directory env variable: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	bot := gopal.NewBot(logger)

	bot.Run()
}
