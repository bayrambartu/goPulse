package notification

import (
	"encoding/json"
	"fmt"
	"gopulse/internal/mq"
	"log"
)

type EmailConsumer struct {
	conn         *mq.Connection
	emailService *EmailService
}

func NewEmailConsumer(conn *mq.Connection, emailService *EmailService) *EmailConsumer {
	return &EmailConsumer{
		conn:         conn,
		emailService: emailService,
	}
}

func (c *EmailConsumer) Start() error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open consumer channel: %w", err)
	}

	defer ch.Close()
	err = ch.Qos(
		1,     // prefetch count , ayni anda en fazla 1 tane ack'lenmemis mesaj
		0,     // prefetch size
		false, // global
	)

	deliveries, err := ch.Consume(
		EmailQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}
	log.Println("Email consumer started, waiting for messages...")

	for d := range deliveries {
		var msg CredentialsEmailMessage

		if err := json.Unmarshal(d.Body, &msg); err != nil {
			log.Printf("failed to unmarshal message, discarding: %v", err)
			d.Nack(false, false)
			continue
		}

		err := c.emailService.SendCredentials(msg.Email, msg.Password)
		if err != nil {
			log.Printf("failed to send email to %s: %v, requeueing", msg.Email, err)

			d.Nack(false, true) // kuruktaki mesajı isleyemedik geri kuyruga koy. anlamında kullanılmaktadir.
			continue
		}
		d.Ack(false)
	}
	return nil

}
