package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	BaseURL string
	Model   string
	APIKey  string
}

func NewConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading env file: %v\n", err)
	}

	return &Config{
		BaseURL: os.Getenv("BASE_URL"),
		Model:   os.Getenv("MODEL"),
		APIKey:  os.Getenv("API_KEY"),
	}
}
