package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"gopulse/internal/user"
	"math/big"
	mrand "math/rand"
	"strings"
	"time"

	"gopulse/internal/config"

	database "gopulse/utils/DB"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	DB     *sql.DB
	Config config.Config
}

// type User struct {
// 	Name    string `json:"name" binding:"required,min=2"`
// 	Surname string `json:"surname" binding:"required,min=2"`
// }

type CredentialsResponse struct {
	Message        string    `json:"message"`
	User           user.User `json:"user"`
	Email          string    `json:"email"`
	Password       string    `json:"password"`
	HashedPassword string    `json:"hashed_password"`
	Verified       bool      `json:"verified"`
}

type LoginCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading .env file:", err)
		return
	}
	cfg := config.Load()

	db := database.ConnectionPostgres(cfg)
	defer db.Close()

	handler := &Handler{
		DB:     db,
		Config: cfg,
	}

	r := gin.Default()

	r.POST("/users", handler.UsersHandler)
	r.POST("/auth/login", handler.Login)

	protected := r.Group("/")
	protected.Use(handler.AuthMiddleware())

	protected.GET("/profile", handler.Profile)

	r.Run(":8080")
}

func GenerateJWT(cfg config.Config, email, name, surname string) (string, error) {
	claims := jwt.MapClaims{
		"email":   email,
		"name":    name,
		"surname": surname,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(cfg.JWTSecret))
}

func (h *Handler) Login(c *gin.Context) {
	loginCredentials := LoginCredentials{}

	if err := c.ShouldBindJSON(&loginCredentials); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var storedHash string

	err := h.DB.QueryRow(
		`SELECT hashed_password FROM kullanicilar WHERE email = $1`,
		loginCredentials.Email,
	).Scan(&storedHash)

	if err != nil {
		c.JSON(400, gin.H{"error": "User not found"})
		return
	}

	if !VerifyPassword(loginCredentials.Password, storedHash) {
		c.JSON(400, gin.H{"error": "Invalid password"})
		return
	}

	var name, surname string

	err = h.DB.QueryRow(
		`SELECT name, surname FROM kullanicilar WHERE email = $1`,
		loginCredentials.Email,
	).Scan(&name, &surname)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to retrieve user information"})
		return
	}

	token, err := GenerateJWT(
		h.Config,
		loginCredentials.Email,
		name,
		surname,
	)

	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(200, gin.H{
		"message":      "Login successful",
		"access_token": token,
		"token_type":   "Bearer",
		"email":        loginCredentials.Email,
		"name":         name,
		"surname":      surname,
	})
}

func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			c.JSON(401, gin.H{
				"error": "Authorization header is required",
			})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")

		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(401, gin.H{
				"error": "Invalid Authorization header format",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {

			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf(
					"unexpected signing method: %v",
					token.Header["alg"],
				)
			}

			return []byte(h.Config.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(401, gin.H{
				"error": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)

		if !ok {
			c.JSON(401, gin.H{
				"error": "Invalid token claims",
			})
			c.Abort()
			return
		}

		email, ok := claims["email"].(string)

		if !ok {
			c.JSON(401, gin.H{
				"error": "Invalid token",
			})
			c.Abort()
			return
		}

		c.Set("userEmail", email)

		c.Next()
	}
}

func (h *Handler) Profile(c *gin.Context) {
	email, exists := c.Get("userEmail")

	if !exists {
		c.JSON(401, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(200, gin.H{
		"email": email,
	})
}

func (h *Handler) UsersHandler(c *gin.Context) {
	user := user.User{}

	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	email, password, err := h.generateCredentials(user.Name, user.Surname)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := HashPassword(password)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to hash password"})
		return
	}

	verified := VerifyPassword(password, hashedPassword)

	res, err := h.DB.Exec(
		`INSERT INTO kullanicilar
		(name, surname, email, password, hashed_password, verified)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		user.Name,
		user.Surname,
		email,
		password,
		hashedPassword,
		verified,
	)

	if err != nil {
		c.JSON(500, gin.H{
			"error": "Failed to insert user into database",
		})
		return
	}

	fmt.Printf("Inserted user into database: %v\n", res)

	c.JSON(200, CredentialsResponse{
		Message:        "User created successfully",
		User:           user,
		Email:          email,
		Password:       password,
		HashedPassword: hashedPassword,
		Verified:       verified,
	})
}

func (h *Handler) generateCredentials(name, surname string) (string, string, error) {
	var email string

	for {
		number := mrand.Intn(90) + 10

		candidateEmail := fmt.Sprintf(
			"%s.%s%d@example.com",
			name,
			surname,
			number,
		)

		var exists bool

		err := h.DB.QueryRow(
			`SELECT EXISTS(
				SELECT 1 FROM kullanicilar WHERE email = $1
			)`,
			candidateEmail,
		).Scan(&exists)

		if err != nil {
			return "", "", fmt.Errorf(
				"error checking email: %w",
				err,
			)
		}

		if !exists {
			email = candidateEmail
			break
		}
	}

	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"

	password := make([]byte, 16)

	for i := range password {
		n, err := rand.Int(
			rand.Reader,
			big.NewInt(int64(len(chars))),
		)

		if err != nil {
			return "", "", fmt.Errorf(
				"error generating random number: %w",
				err,
			)
		}

		password[i] = chars[n.Int64()]
	}

	return email, string(password), nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		14,
	)

	return string(bytes), err
}

func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword(
		[]byte(hash),
		[]byte(password),
	)

	return err == nil
}
