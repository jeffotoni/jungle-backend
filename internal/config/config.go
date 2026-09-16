package config

import (
	"os"
	"time"
)

type Config struct {
	AppEnv string

	HTTPAddr string
	LogFormat string

	DatabaseURL string

	AWSRegion string
	AWSEndpointURL string
	SQSWagerQueueURL string
	SQSEventsQueueURL string

	KeycloakBaseURL string
	KeycloakRealm string
	KeycloakClientID string
	KeycloakClientSecret string

	ShutdownTimeout time.Duration
}

func Load() Config {
	return Config{
		AppEnv: env("APP_ENV", "local"),

		HTTPAddr: env("HTTP_ADDR", ":8080"),
		LogFormat: env("LOG_FORMAT", "json"),

		DatabaseURL: env("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable"),

		AWSRegion: env("AWS_REGION", "us-east-1"),
		AWSEndpointURL: env("AWS_ENDPOINT_URL", "http://localhost:4566"),
		SQSWagerQueueURL: os.Getenv("SQS_WAGER_QUEUE_URL"),
		SQSEventsQueueURL: os.Getenv("SQS_EVENTS_QUEUE_URL"),

		KeycloakBaseURL: env("KEYCLOAK_BASE_URL", "http://localhost:8081"),
		KeycloakRealm: env("KEYCLOAK_REALM", "jungle"),
		KeycloakClientID: env("KEYCLOAK_CLIENT_ID", "jungle-api"),
		KeycloakClientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),

		ShutdownTimeout: 10 * time.Second,
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
