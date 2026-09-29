package mq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

func DeclareQueueWithDLX(ch *amqp.Channel, queueName string, dlxExchangeName string, dlqName string) (amqp.Queue, error) {

	err := ch.ExchangeDeclare(
		dlxExchangeName,
		"direct",
		true,  // durable
		false, // autoDelete
		false, // internal
		false, // noWait
		nil,
	)
	if err != nil {
		return amqp.Queue{}, err
	}

	dlq, err := ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		return amqp.Queue{}, err
	}
	err = ch.QueueBind(dlq.Name, dlq.Name, dlxExchangeName, false, nil)
	if err != nil {
		return amqp.Queue{}, err
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    dlxExchangeName,
		"x-dead-letter-routing-key": dlq.Name,
	}
	return ch.QueueDeclare(queueName, true, false, false, false, args)
}
