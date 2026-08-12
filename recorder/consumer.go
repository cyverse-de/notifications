package recorder

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

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

// requeueDelay is how long a delivery is held onto before it's requeued. Requeueing immediately
// turns a failure that keeps recurring, such as an unreachable database, into a hot loop that pegs
// a CPU and floods the logs.
const requeueDelay = 5 * time.Second

// Consumer dispatches incoming AMQP deliveries to a Recorder.
type Consumer struct {
	amqpClient   *messaging.Client
	publisher    MessagingClient
	amqpSettings *common.AMQPSettings
	supportEmail string
	recorder     *Recorder
	inFlight     atomic.Int64
}

// NewConsumer creates a consumer that records deliveries from the event queue. The AMQP client is
// supplied by the caller so that the process owns every connection's lifetime. The publisher is a
// separate client because a failure on the connection used to publish must not disrupt consumption.
func NewConsumer(
	amqpClient *messaging.Client,
	publisher MessagingClient,
	amqpSettings *common.AMQPSettings,
	supportEmail string,
	recorder *Recorder,
) *Consumer {
	return &Consumer{
		amqpClient:   amqpClient,
		publisher:    publisher,
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

// requeue holds onto a delivery for a while and then returns it to the queue to be retried.
func (c *Consumer) requeue(ctx context.Context, delivery amqp.Delivery) {
	select {
	case <-time.After(requeueDelay):
	case <-ctx.Done():
	}
	c.nack(delivery, true)
}

// recoverFromPanic keeps a defect in the recording path from taking down the process, which also
// serves the HTTP API. The delivery is discarded because a panic would recur on every redelivery.
func (c *Consumer) recoverFromPanic(ctx context.Context, delivery amqp.Delivery) {
	r := recover()
	if r == nil {
		return
	}

	cause := NewUnrecoverableError("panic while recording a notification event: %v\n%s", r, debug.Stack())
	log.Error(cause.Error())
	c.sendUnrecoverableErrorEmail(ctx, delivery, cause)
	c.logDelivery("discarded delivery", delivery)
	c.nack(delivery, false)
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
	err := c.publisher.PublishEmailRequestContext(ctx, &request)
	if err != nil {
		log.Errorf("%s: %s", wrapMsg, err.Error())
	}
}

// logDelivery logs some information about a message delivery for troubleshooting purposes. The
// message body is only logged at debug level because it contains the recipient's email address.
func (c *Consumer) logDelivery(description string, delivery amqp.Delivery) {
	log.Infof("%s: %s", description, delivery.RoutingKey)
	log.Debugf("%s: %s; %s", description, delivery.RoutingKey, delivery.Body)
}

// Drain waits up to timeout for in-flight deliveries to finish. Recorder.Record publishes the
// email and UI messages after it commits, so closing the connections mid-delivery would leave a
// recorded notification that the user is never pinged about.
func (c *Consumer) Drain(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for c.inFlight.Load() > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n := c.inFlight.Load(); n > 0 {
		log.Warnf(
			"%d notification events still in flight after the %s drain window; "+
				"any that already committed may not have their email or UI message published",
			n, timeout,
		)
	}
}

// handleMessage handles an incoming AMQP message.
func (c *Consumer) handleMessage(ctx context.Context, delivery amqp.Delivery) {
	c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	defer c.recoverFromPanic(ctx, delivery)

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
			c.requeue(ctx, delivery)
		default:
			log.Errorf(
				"requeuing message because of an error that is presumed to be recoverable: %s",
				err.Error(),
			)
			c.logDelivery("requeued delivery", delivery)
			c.requeue(ctx, delivery)
		}
		return
	}

	// If we get here then the delivery was processed successfully.
	c.ack(delivery)
}

// Listen waits for incoming AMQP messages and dispatches any that it receives to the recorder.
func (c *Consumer) Listen() error {
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

	// AddConsumer reports neither a failed declaration nor a failed registration, so check that the
	// queue reached the broker rather than reporting a successful start on the strength of a call
	// that can't fail. A mismatched exchange type, for instance, fails the declarations and leaves
	// nothing consuming the queue.
	queueExists, err := c.amqpClient.QueueExists(QueueName, true, false)
	if err != nil {
		return fmt.Errorf("unable to verify that the %s queue exists: %w", QueueName, err)
	}
	if !queueExists {
		return fmt.Errorf("the %s queue was not declared; check the AMQP exchange settings", QueueName)
	}

	return nil
}
