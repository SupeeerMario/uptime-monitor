package config

import (
	"log"
	"os"
	"strconv"
	// "github.com/joho/godotenv"
)

type Config struct {
	DB_URL       string
	PORT         string
	PROBER_COUNT int
}

func Load() Config {
	var count int

	//	_ = godotenv.Load()

	if val, ok := os.LookupEnv("DB_URL"); ok == false || val == "" {
		log.Fatal("ERROR LOADING DB_URL FROM .ENV")
	}
	if val, ok := os.LookupEnv("PORT"); ok == false || val == "" {
		log.Fatal("ERROR LOADING PORT FROM .ENV")
	}
	if val, ok := os.LookupEnv("PROBER_COUNT"); ok == false || val == "" {
		log.Fatal("ERROR LOADING PROBER_COUNT FROM .ENV")
	}

	if val, err := strconv.Atoi(os.Getenv("PORT")); err != nil || val <= 0 {
		log.Fatalf("Invalid Port Var %v", err)
	}

	if val, err := strconv.Atoi(os.Getenv("PROBER_COUNT")); err != nil || val <= 0 {
		log.Fatalf("Invalid Prober_Count: %q", os.Getenv("PROBER_COUNT"))
	} else {
		count = val
	}

	return Config{

		DB_URL:       os.Getenv("DB_URL"),
		PORT:         os.Getenv("PORT"),
		PROBER_COUNT: count,
	}
}
