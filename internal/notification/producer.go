package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"gopulse/internal/mq"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const EmailQueueName = "email.send_queue"

type CredentialsEmailMessage struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailProducer struct {
	conn *mq.Connection
}

func NewEmailProducer(conn *mq.Connection) *EmailProducer {
	return &EmailProducer{conn: conn}
}
func (p *EmailProducer) Publish(email, password string) error {

	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %w", err)
	}
	defer ch.Close()

	msg := CredentialsEmailMessage{Email: email, Password: password}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal email message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return ch.PublishWithContext(
		ctx,
		"",             // exchange: "" = default exchange
		EmailQueueName, // routing key: default exchange'de = hedef queue adı
		false,          // mandatory: queue bulunamazsa sessizce mi düşsün (false=evet)
		false,          // immediate: (deprecated, hep false bırak)
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		},
	)
}
