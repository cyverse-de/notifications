package recorder

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/common"
	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueName and RoutingKey must match what the retired event-recorder service used, so that
// during a rollout both it and this service are competing consumers on the same queue rather
// than each building their own.
const QueueName = "event_listener"
const RoutingKey = "events.*.update.*"

// eventCategory is the routing key category this service records. The binding admits
// events.*.update.*, but notifications are the only category anything publishes.
const eventCategory = "notification"

// prefetchCount is the number of unacknowledged deliveries the broker will hand this consumer.
const prefetchCount = 100

// Consumer dispatches incoming AMQP deliveries to a Recorder.
type Consumer struct {
	amqpClient   *messaging.Client
	amqpSettings *common.AMQPSettings
	supportEmail string
	recorder     *Recorder
}

// NewConsumer creates a consumer that records deliveries from the event queue. The AMQP client
// is supplied by the caller so that the process owns every connection's lifetime.
func NewConsumer(
	amqpClient *messaging.Client,
	amqpSettings *common.AMQPSettings,
	supportEmail string,
	recorder *Recorder,
) *Consumer {
	return &Consumer{
		amqpClient:   amqpClient,
		amqpSettings: amqpSettings,
		supportEmail: supportEmail,
		recorder:     recorder,
	}
}

// parseRoutingKey extracts the event category and update type from the delivery tag.
func (c *Consumer) parseRoutingKey(tag string) (string, string, error) {
	components := strings.Split(tag, ".")
	if len(components) < 4 {
		return "", "", fmt.Errorf("routing key %s has too few components", tag)
	}
	return components[1], components[3], nil
}

// ack acknowledges a delivery and logs an error if the acknowledgement fails.
func (c *Consumer) ack(delivery amqp.Delivery) {
	err := delivery.Ack(false)
	if err != nil {
		log.Errorf("unable to acknowledge delivery: %s", err.Error())
	}
}

// nack negatively acknowledges a delivery and logs an error if the acknowledgement fails.
func (c *Consumer) nack(delivery amqp.Delivery, requeue bool) {
	err := delivery.Nack(false, requeue)
	if err != nil {
		log.Errorf("unable to negatively acknowledge delivery: %s", err.Error())
	}
}

// sendUnrecoverableErrorEmail sends an email to a configurable email address indicating that
// a message delivery couldn't be processed.
func (c *Consumer) sendUnrecoverableErrorEmail(ctx context.Context, delivery amqp.Delivery, cause UnrecoverableError) {
	wrapMsg := "unable to send unrecoverable error notification email request"

	// Build the email request.
	request := messaging.EmailRequest{
		Subject:      "Unrecoverable Error Recording a Notification Event",
		ToAddress:    c.supportEmail,
		TemplateName: "notifications_event_discarded",
		TemplateValues: map[string]interface{}{
			"error":        cause.Error(),
			"routing_key":  delivery.RoutingKey,
			"message_body": string(delivery.Body),
		},
	}

	// Publish the request.
	err := c.amqpClient.PublishEmailRequestContext(ctx, &request)
	if err != nil {
		log.Errorf("%s: %s", wrapMsg, err.Error())
	}
}

// logDelivery logs some information about a message delivery for troubleshooting purposes.
func (c *Consumer) logDelivery(description string, delivery amqp.Delivery) {
	log.Infof("%s: %s; %s", description, delivery.RoutingKey, delivery.Body)
}

// handleMessage handles an incoming AMQP message.
func (c *Consumer) handleMessage(ctx context.Context, delivery amqp.Delivery) {
	category, updateType, err := c.parseRoutingKey(delivery.RoutingKey)
	if err != nil {
		log.Errorf("unable to handle message: %s", err.Error())
		c.nack(delivery, false)
		return
	}

	// The binding admits every event category, but notifications are the only one recorded.
	if category != eventCategory {
		log.Infof("no handler for category '%s'; ignoring delivery", category)
		c.ack(delivery)
		return
	}

	// Dispatch the delivery to the recorder.
	err = c.recorder.Record(ctx, updateType, delivery.Body, delivery.RoutingKey)
	if err != nil {
		var unrecoverable UnrecoverableError
		var recoverable RecoverableError
		switch {
		case errors.As(err, &unrecoverable):
			log.Errorf("discarding message because of an unrecoverable error: %s", err.Error())
			c.sendUnrecoverableErrorEmail(ctx, delivery, unrecoverable)
			c.logDelivery("discarded delivery", delivery)
			c.nack(delivery, false)
		case errors.As(err, &recoverable):
			log.Errorf("requeuing message because of a recoverable error: %s", err.Error())
			c.logDelivery("requeued delivery", delivery)
			c.nack(delivery, true)
		default:
			log.Errorf(
				"requeuing message because of an error that is presumed to be recoverable: %s",
				err.Error(),
			)
			c.logDelivery("requeued delivery", delivery)
			c.nack(delivery, true)
		}
		return
	}

	// If we get here then the delivery was processed successfully.
	c.ack(delivery)
}

// Listen waits for incoming AMQP messages and dispatches any that it receives to the recorder.
func (c *Consumer) Listen() error {
	// Set up publishing on the AMQP client, which the unrecoverable-error email needs.
	if err := c.amqpClient.SetupPublishing(c.amqpSettings.ExchangeName); err != nil {
		return fmt.Errorf("unable to set up publishing on the consumer client: %w", err)
	}

	// Start listening for incoming messages.
	go c.amqpClient.Listen()

	c.amqpClient.AddConsumer(
		c.amqpSettings.ExchangeName,
		c.amqpSettings.ExchangeType,
		QueueName,
		RoutingKey,
		c.handleMessage,
		prefetchCount,
	)

	return nil
}
