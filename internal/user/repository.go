package user

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var ErrEmailAlreadyExists = fmt.Errorf("email already exists")

type UserRepository interface {
	Create(user User) error
	//EmailExists(email string) (bool, error)
	FindHashedPasswordByEmail(email string) (string, error)
	FindNameSurnameByEmail(email string) (name string, surname string, err error)
}

type PostgresUserRepository struct {
	DB *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{DB: db}
}

func (r *PostgresUserRepository) Create(user User) error {
	_, err := r.DB.Exec(
		`INSERT INTO kullanicilar (name, surname, email, hashed_password, verified)
		 VALUES ($1, $2, $3,  $4, $5)`,
		user.Name, user.Surname, user.Email, user.HashedPassword, user.Verified,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrEmailAlreadyExists
		}
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *PostgresUserRepository) EmailExists(email string) (bool, error) {
	var exists bool
	err := r.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM kullanicilar WHERE email = $1)`,
		email,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}
	return exists, nil
}
func (r *PostgresUserRepository) FindHashedPasswordByEmail(email string) (string, error) {
	var hashedPassword string
	err := r.DB.QueryRow(
		`SELECT hashed_password FROM kullanicilar WHERE email = $1`,
		email,
	).Scan(&hashedPassword)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	return hashedPassword, nil
}
func (r *PostgresUserRepository) FindNameSurnameByEmail(email string) (string, string, error) {
	var name, surname string
	err := r.DB.QueryRow(
		`SELECT name, surname FROM kullanicilar WHERE email = $1`,
		email,
	).Scan(&name, &surname)
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve user info: %w", err)
	}
	return name, surname, nil
}
