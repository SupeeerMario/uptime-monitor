package config

import (
	"log"
	"os"
	"strconv"

//	"github.com/joho/godotenv"
)

type Config struct {
	DB_URL string
	PORT   string
}

func Load() Config {

//	_ = godotenv.Load()

	if val, ok := os.LookupEnv("DB_URL"); ok == false || val == "" {
		log.Fatal("ERROR LOADING DB_URL FROM .ENV")
	}
	if val, ok := os.LookupEnv("PORT"); ok == false || val == "" {
		log.Fatal("ERROR LOADING PORT FROM .ENV")
	}

	if val, err := strconv.Atoi(os.Getenv("PORT")); err != nil || val == 0 {
		log.Fatalf("Invalid Port Var %v", err)
	}

	return Config{

		DB_URL: os.Getenv("DB_URL"),
		PORT:   os.Getenv("PORT"),
	}
}
