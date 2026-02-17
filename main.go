package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/jlry-dev/gopal/bot"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("failed to load env field")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	bot := bot.NewBot(logger)

	bot.Run()
}
