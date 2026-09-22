package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"gopulse/internal/mq"
	"gopulse/internal/notification"
	"gopulse/internal/user"
	"log"
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
	UserRepository user.UserRepository
	EmailProducer  *notification.EmailProducer
	Config         config.Config
}

type CredentialsResponse struct {
	Message        string `json:"message"`
	Name           string `json:"name"`
	Surname        string `json:"surname"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	HashedPassword string `json:"hashed_password"`
	Verified       bool   `json:"verified"`
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

	mqConn, err := mq.Connect(cfg.RabbitMQURL)
	if err != nil {
		fmt.Println("Error connecting to RabbitMQ:", err)
		return
	}
	defer mq.Close(mqConn)

	setupCh, err := mqConn.Channel()
	if err != nil {
		log.Fatal("Failed to open setup channel:", err)
	}

	if _, err := mq.DeclareQueue(setupCh, notification.EmailQueueName); err != nil {
		log.Fatal("Failed to declare email queue:", err)
	}
	setupCh.Close()

	emailProducer := notification.NewEmailProducer(mqConn)

	userRepository := user.NewPostgresUserRepository(db)

	EmailService := notification.NewEmailService()
	EmailConsumer := notification.NewEmailConsumer(mqConn, EmailService)

	go func() {
		if err := EmailConsumer.Start(); err != nil {
			log.Fatal("Failed to start email consumer:", err)
		}
	}()
	handler := &Handler{
		UserRepository: userRepository,
		EmailProducer:  emailProducer,
		Config:         cfg,
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

	storedHash, err := h.UserRepository.FindHashedPasswordByEmail(loginCredentials.Email)

	if err != nil {
		c.JSON(400, gin.H{"error": "User not found"})
		return
	}

	if !VerifyPassword(loginCredentials.Password, storedHash) {
		c.JSON(400, gin.H{"error": "Invalid password"})
		return
	}

	var name, surname string

	name, surname, err = h.UserRepository.FindNameSurnameByEmail(loginCredentials.Email)

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
	var req user.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	password, err := GenerateRandomPassword()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate password"})
		return
	}

	hashedPassword, err := HashPassword(password)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to hash password"})
		return
	}

	verified := VerifyPassword(password, hashedPassword)

	const maxRetries = 5
	var email string
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		candidateEmail := generateCandidateEmail(req.Name, req.Surname)

		newUser := user.User{
			Name:           req.Name,
			Surname:        req.Surname,
			Email:          candidateEmail,
			HashedPassword: hashedPassword,
			Verified:       verified,
		}

		err := h.UserRepository.Create(newUser)
		if err == nil {
			email = candidateEmail
			lastErr = nil
			break
		}

		if errors.Is(err, user.ErrEmailAlreadyExists) {
			lastErr = err
			continue
		}

		fmt.Printf("Create user error: %v\n", err)

		c.JSON(500, gin.H{
			"error":   "Failed to insert user into database",
			"details": err.Error(),
		})
		return
	}

	if email == "" {
		fmt.Println("could not generate unique email after retries:", lastErr)
		c.JSON(500, gin.H{"error": "Could not create user, please try again"})
		return
	}

	// if err := h.EmailService.SendCredentials(email, password); err != nil {
	// 	c.JSON(500, gin.H{"error": "User created but failed to send email"})
	// 	return
	// }
	if err := h.EmailProducer.Publish(email, password); err != nil {
		c.JSON(500, gin.H{"error": "User created but failed to queue notification", "created name": req.Name, "created user email": email})
		return
	}
	c.JSON(200, CredentialsResponse{
		Message:        "User created successfully",
		Name:           req.Name,
		Surname:        req.Surname,
		Email:          email,
		Password:       password,
		HashedPassword: hashedPassword,
		Verified:       verified,
	})
}

func generateCandidateEmail(name, surname string) string {
	number := mrand.Intn(90) + 10
	return fmt.Sprintf("%s.%s%d@example.com", name, surname, number)
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

func GenerateRandomPassword() (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
	password := make([]byte, 16)
	for i := range password {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", fmt.Errorf("error generating random number: %w", err)
		}
		password[i] = chars[n.Int64()]
	}
	return string(password), nil
}
