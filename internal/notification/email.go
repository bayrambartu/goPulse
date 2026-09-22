package notification

import (
	"fmt"
	"math/rand"
	"time"
)

type EmailService struct{}

func NewEmailService() *EmailService {
	return &EmailService{}
}

func (s *EmailService) SendCredentials(email, password string) error {
	delay := time.Duration(300+rand.Intn(1200)) * time.Millisecond
	time.Sleep(delay)

	if rand.Intn(100) < 20 {
		return fmt.Errorf("email service temporarily unavailable")
	}

	fmt.Printf("[EMAIL SENT] to=%s password=%s (took %v)\n", email, password, delay)

	return nil
}
