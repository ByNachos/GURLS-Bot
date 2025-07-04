package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

// Config holds all the configuration for the application.
type Config struct {
	Env        string `yaml:"env" env:"ENV" env-default:"production"`
	Telegram   `yaml:"telegram"`
	GRPCClient `yaml:"grpc_client"`
	HTTPServer `yaml:"http_server"`
}

// Telegram holds Telegram specific configuration.
type Telegram struct {
	Token string `yaml:"token" env:"TELEGRAM_TOKEN" env-required:"true"`
}

// GRPCClient holds gRPC client specific configuration.
type GRPCClient struct {
	BackendAddress string        `yaml:"backend_address" env:"GRPC_BACKEND_ADDRESS" env-default:"localhost:50051"`
	Timeout        time.Duration `yaml:"timeout" env:"GRPC_CLIENT_TIMEOUT" env-default:"5s"`
}

// HTTPServer holds HTTP server configuration (for base URL generation).
type HTTPServer struct {
	BaseURL string `yaml:"base_url" env:"BASE_URL" env-default:"http://localhost:8080"`
}

// MustLoad loads the application configuration.
func MustLoad() *Config {
	// Try to load .env file (ignore error in production)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment variables")
	}

	var cfg Config

	// Check if config file path is specified
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/local.yml" // default path
	}

	// Try to load config file
	if _, err := os.Stat(configPath); err == nil {
		if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
			log.Fatalf("cannot read config: %s", err)
		}
	} else {
		// If config file doesn't exist, use environment variables only
		log.Println("Config file not found, using environment variables only")
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			log.Fatalf("cannot read config from environment: %s", err)
		}
	}

	return &cfg
}