package config

import (
	"os"
)

type Config struct {
	DBHost, DBUser, DBPassword, DBName, DBPort, JWTSecret, RabbitMQURL string
}

func Load() Config {
	return Config{
		DBHost:      os.Getenv("DB_HOST"),
		DBUser:      os.Getenv("DB_USER"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBName:      os.Getenv("DB_NAME"),
		DBPort:      os.Getenv("DB_PORT"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL")}

}
