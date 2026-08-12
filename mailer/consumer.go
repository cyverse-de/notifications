package mailer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/common"
	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueName must match what the retired de-mailer service used, so that during a rollout both
// it and this service are competing consumers on the same queue rather than each building
// their own. The recorder in this same service is the only publisher on the routing key the
// queue is bound to; the hop through the broker is kept so that a send that fails mid-flight
// can be retried from the queue rather than lost.
const QueueName = "email_requests"

// prefetchCount bounds how many unacked deliveries the broker will hand this consumer at once.
const prefetchCount = 100

// Consumer consumes email requests from AMQP and processes them in-process.
type Consumer struct {
	client    *messaging.Client
	settings  *common.AMQPSettings
	processor *EmailProcessor
	inFlight  atomic.Int64
}

// NewConsumer creates a new email request consumer with reconnection enabled.
func NewConsumer(processor *EmailProcessor, settings *common.AMQPSettings) (*Consumer, error) {
	client, err := messaging.NewClient(settings.URI, true)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to AMQP; this usually means the broker is down or the URI is wrong: %w", err)
	}
	return &Consumer{
		client:    client,
		settings:  settings,
		processor: processor,
	}, nil
}

// Listen starts consuming email requests from the durable email_requests queue.
func (c *Consumer) Listen() {
	go c.client.Listen()
	c.client.AddConsumer(
		c.settings.ExchangeName,
		c.settings.ExchangeType,
		QueueName,
		messaging.EmailRequestPublishingKey,
		c.handleMessage,
		prefetchCount,
	)
}

// Drain waits up to timeout for in-flight deliveries to finish processing and ack, so emails
// that were already sent aren't requeued (and re-sent) when the connection closes.
func (c *Consumer) Drain(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for c.inFlight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n := c.inFlight.Load(); n > 0 {
		log.Warnf("%d email requests still in flight after the %s drain window; their deliveries will be requeued and may be sent twice", n, timeout)
	}
}

// Close shuts down the AMQP connection; unacknowledged deliveries are requeued by the broker.
func (c *Consumer) Close() {
	c.client.Close()
}

// Shutdown drains in-flight deliveries and then closes the connection.
func (c *Consumer) Shutdown(timeout time.Duration) {
	c.Drain(timeout)
	c.Close()
}

// handleMessage processes a single delivery. Failures are logged and the message is acked
// anyway: a failed request would almost certainly fail again on redelivery, and dropping it
// matches the behavior of the retired de-mailer service.
func (c *Consumer) handleMessage(ctx context.Context, delivery amqp.Delivery) {
	c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	alog := log.WithContext(ctx).WithField("transport", "amqp")
	if err := c.processor.Process(ctx, delivery.Body); err != nil {
		alog.Errorf("failed to process email request; the message will be dropped: %s", err)
	}
	if err := delivery.Ack(false); err != nil {
		alog.Errorf("failed to ack email request message: %s", err)
	}
}

// verifyTopology declares the exchange, queue, and binding on a short-lived connection,
// mirroring the messaging library's own declarations. The library silently discards
// declaration errors during consumer setup, so this is the only place a broker/config mismatch
// (e.g. a wrong exchange type) surfaces as an error instead of a silent no-consumer.
func verifyTopology(settings *common.AMQPSettings) error {
	conn, err := amqp.Dial(settings.URI)
	if err != nil {
		return fmt.Errorf("unable to connect to the AMQP broker: %w", err)
	}
	defer conn.Close() // nolint:errcheck

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("unable to open an AMQP channel: %w", err)
	}
	defer ch.Close() // nolint:errcheck

	if err := ch.ExchangeDeclare(settings.ExchangeName, settings.ExchangeType, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declaring exchange %q (type %q) failed; this usually means amqp.exchange.type doesn't match the existing exchange: %w",
			settings.ExchangeName, settings.ExchangeType, err)
	}
	if _, err := ch.QueueDeclare(QueueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declaring queue %q failed; this usually means the existing queue was declared with different arguments: %w",
			QueueName, err)
	}
	if err := ch.QueueBind(QueueName, messaging.EmailRequestPublishingKey, settings.ExchangeName, false, nil); err != nil {
		return fmt.Errorf("binding queue %q to exchange %q failed: %w", QueueName, settings.ExchangeName, err)
	}
	return nil
}

// StartConsumer verifies the AMQP topology (retrying with backoff so a broker outage never
// blocks or kills HTTP delivery), then starts the consumer. It returns nil if ctx is canceled
// before the broker becomes reachable.
func StartConsumer(ctx context.Context, processor *EmailProcessor, settings *common.AMQPSettings) *Consumer {
	backoff := time.Second
	for {
		err := verifyTopology(settings)
		if err == nil {
			break
		}
		log.Errorf("AMQP consumer setup failed, retrying in %s (HTTP email delivery is unaffected): %s", backoff, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}

	consumer, err := NewConsumer(processor, settings)
	if err != nil {
		log.Errorf("unable to create the email request consumer; AMQP email requests will not be consumed: %s", err)
		return nil
	}
	consumer.Listen()
	log.Infof("consuming AMQP queue %s", QueueName)
	return consumer
}
