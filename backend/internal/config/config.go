package config

import "os"

type Config struct {
	Port         string
	DatabasePath string
	JWTSecret    string
	FrontendURL  string
	SeedData     bool
	GoogleAPIKey string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		DatabasePath: getEnv("DATABASE_PATH", "./tripplanner.db"),
		JWTSecret:    getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		FrontendURL:  getEnv("FRONTEND_URL", "http://localhost:5173"),
		SeedData:     getEnvBool("SEED_DATA", true),
		GoogleAPIKey: getEnv("API_KEY", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	switch os.Getenv(key) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return defaultValue
}
