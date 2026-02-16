package main

import (
	"log"

	"github.com/jlry-dev/gopal/bot"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("failed to load env field")
	}

	bot.Run()
}
