package db

import (
	"database/sql"
	"gopulse/internal/config"
	"log"

	_ "github.com/lib/pq"
)

func ConnectionPostgres(cfg config.Config) *sql.DB {

	dsn := "host=" + cfg.DBHost + " port=" + cfg.DBPort + " user=" + cfg.DBUser + " password=" + cfg.DBPassword + " dbname=" + cfg.DBName + " sslmode=disable"

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Connection error:", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("Ping error:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS kullanicilar (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    surname VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    hashed_password VARCHAR(255) NOT NULL,
    verified BOOLEAN NOT NULL DEFAULT false
)`)
	if err != nil {
		log.Fatal("Table creation error:", err)
	}

	return db
}
