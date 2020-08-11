package common

// AMQPSettings represents the settings that we require in order to connect to the AMQP exchange.
type AMQPSettings struct {
	URI          string
	ExchangeName string
	ExchangeType string
}
