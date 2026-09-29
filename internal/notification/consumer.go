package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"gopulse/internal/mq"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

const maxRetries = 3

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

	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}
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
		retryCount := getRetryCount(d.Headers)

		log.Printf("received message for %s (retry=%d), simulating crash...", msg.Email, retryCount)
		// os.Exit(1)

		err := c.emailService.SendCredentials(msg.Email, msg.Password)
		if err == nil {
			d.Ack(false)
			continue
		}

		if retryCount >= maxRetries {
			log.Printf(
				"message for %s failed after %d attempts, giving up (dropping message)",
				msg.Email, retryCount,
			)
			d.Nack(false, false)
			continue
		}

		log.Printf(
			"message for %s failed (attempt %d/%d), retrying: %v",
			msg.Email, retryCount+1, maxRetries, err,
		)

		if pubErr := c.republishWithIncrementedRetry(d, retryCount); pubErr != nil {
			log.Printf("failed to republish with retry count, falling back to requeue: %v", pubErr)
			d.Nack(false, true)
			continue
		}
		d.Ack(false)
	}

	return nil
}
func getRetryCount(headers amqp.Table) int {

	if headers == nil {
		return 0
	}
	if v, ok := headers["x-retry-count"]; ok {
		if count, ok := v.(int32); ok {
			return int(count)
		}
	}
	return 0
}

func (c *EmailConsumer) republishWithIncrementedRetry(d amqp.Delivery, currentRetryCount int) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	newHeaders := amqp.Table{"x-retry-count": int32(currentRetryCount + 1)}

	return ch.PublishWithContext(
		context.Background(),
		"",
		EmailQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  d.ContentType,
			Body:         d.Body,
			DeliveryMode: amqp.Persistent,
			Headers:      newHeaders,
		},
	)
}
