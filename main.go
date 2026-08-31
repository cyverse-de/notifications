package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DavidGamba/go-getoptions"
	"github.com/cyverse-de/configurate"
	"github.com/cyverse-de/echo-middleware/v2/redoc"
	"github.com/cyverse-de/go-mod/otelutils"
	"github.com/cyverse-de/messaging/v12"
	"github.com/cyverse-de/notifications/api"
	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/db"
	"github.com/cyverse-de/notifications/mailer"
	"github.com/cyverse-de/notifications/recorder"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	_ "github.com/lib/pq"
)

const serviceName = "notifications"

// The shutdown budget has to fit inside the pod's termination grace period, which the deployment
// leaves at Kubernetes' 30 second default. The three drains run concurrently, so the worst case is
// mailerReadyTimeout + drainTimeout rather than the sum of all of them.
const (
	// drainTimeout is how long each AMQP consumer gets to finish its in-flight deliveries.
	drainTimeout = 15 * time.Second

	// httpShutdownTimeout is how long the HTTP server gets to finish its in-flight requests.
	httpShutdownTimeout = 10 * time.Second

	// mailerReadyTimeout bounds the wait for the email request consumer to report whether it
	// started, for the case where SIGTERM arrives while it's still retrying a broker connection.
	mailerReadyTimeout = 5 * time.Second
)

// commandLineOptionValues represents the values of the options that were passed on the command line when this
// service was invoked.
type commandLineOptionValues struct {
	Config string
	Port   int
	Debug  bool
}

// parseCommandLine parses the command line and returns an options structure containing command-line options and
// parameters.
func parseCommandLine() *commandLineOptionValues {
	optionValues := &commandLineOptionValues{}
	opt := getoptions.New()

	// Default option values.
	defaultConfigPath := "/etc/iplant/de/jobservices.yml"
	defaultPort := 8080

	// Define the command-line options.
	opt.Bool("help", false, opt.Alias("h", "?"))
	opt.StringVar(&optionValues.Config, "config", defaultConfigPath,
		opt.Alias("c"),
		opt.Description("the path to the configuration file"))
	opt.IntVar(&optionValues.Port, "port", defaultPort,
		opt.Alias("p"),
		opt.Description("the TCP port to listen to"))
	opt.BoolVar(&optionValues.Debug, "debug", false,
		opt.Alias("d"),
		opt.Description("enable debug logging"))

	// Parse the command line, handling requests for help and usage errors.
	_, err := opt.Parse(os.Args[1:])
	if opt.Called("help") {
		fmt.Fprint(os.Stderr, opt.Help())
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err)
		fmt.Fprint(os.Stderr, opt.Help(getoptions.HelpSynopsis))
		os.Exit(1)
	}

	return optionValues
}

// buildLoggerEntry sets some logging options then returns a logger entry with some custom fields
// for convenience.
func buildLoggerEntry(optionValues *commandLineOptionValues) *logrus.Entry {

	// Enable logging the file name and line number.
	logrus.SetReportCaller(true)

	// Set the logging format to JSON for now because that's what Echo's middleware uses.
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Enable debugging if we're supposed to.
	if optionValues.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Return the custom log entry.
	return logrus.WithFields(logrus.Fields{
		"service": serviceName,
		"art-id":  serviceName,
		"group":   "org.cyverse",
	})
}

// CustomValidator represents a validator that Echo can use to check incoming requests.
type CustomValidator struct {
	validator *validator.Validate
}

// Validate performs validation for an incoming request.
func (cv CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

// createMessagingClient creates a new AMQP messaging client and sets up publishing on that client.
func createMessagingClient(amqpSettings *common.AMQPSettings) (*messaging.Client, error) {
	wrapMsg := "unable to create the messaging client"

	// Create the messaging client.
	client, err := messaging.NewClient(amqpSettings.URI, true)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Set up publishing on the messaging client.
	err = client.SetupPublishing(amqpSettings.ExchangeName)
	if err != nil {
		client.Close()
		return nil, errors.Wrap(err, wrapMsg)
	}

	return client, nil
}

// requiredConfigKeys lists the settings that the service can't run correctly without. Only
// `amqp.uri` has a built-in default, so the rest silently resolve to the empty string when they're
// absent from the configuration file, and the config file itself is optional.
var requiredConfigKeys = []string{
	"amqp.uri",
	"amqp.exchange.name",
	"amqp.exchange.type",
	"notifications.db.uri",
	"notifications.uid.domain",
	"email.request",
	"email.fromAddress",
	"email.smtpHost",
	"de.base",
}

// validateConfig returns an error naming every required setting that's missing from the
// configuration. Every missing setting is reported at once so that a misconfigured deployment can
// be corrected in a single pass.
func validateConfig(cfg *viper.Viper) error {
	var missing []string
	for _, key := range requiredConfigKeys {
		if strings.TrimSpace(cfg.GetString(key)) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration settings: %s", strings.Join(missing, ", "))
	}

	return nil
}

func main() {
	optionValues := parseCommandLine()

	log := buildLoggerEntry(optionValues)

	var tracerCtx, cancel = context.WithCancel(context.Background())
	defer cancel()
	shutdown := otelutils.TracerProviderFromEnv(tracerCtx, serviceName, func(e error) { log.Fatal(e) })
	defer shutdown()

	// Create the web server.
	e := echo.New()

	// Set a custom logger.
	e.Logger = Logger{Entry: log}

	// Register a custom validator.
	e.Validator = &CustomValidator{validator: validator.New()}

	// Add middleware.
	e.Use(otelecho.Middleware(serviceName))
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(redoc.Serve(redoc.Opts{Title: "DE Notifications API Documentation"}))

	// Load the service information from the Swagger JSON.
	e.Logger.Info("loading service information")
	serviceInfo, err := getSwaggerServiceInfo()
	if err != nil {
		e.Logger.Fatal(err)
	}

	// Load the configuration.
	e.Logger.Info("loading the configuration file")
	cfg, err := configurate.InitDefaults(optionValues.Config, configurate.JobServicesDefaults)
	if err != nil {
		e.Logger.Fatalf("unable to load the configuration file: %s", err.Error())
	}

	// Check the configuration before anything tries to connect, so that a missing setting is
	// reported as a startup failure rather than surfacing later as a connection or delivery error.
	if err = validateConfig(cfg); err != nil {
		e.Logger.Fatalf("invalid configuration: %s", err.Error())
	}

	// Retrieve the AMQP settings.
	amqpSettings := &common.AMQPSettings{
		URI:          cfg.GetString("amqp.uri"),
		ExchangeName: cfg.GetString("amqp.exchange.name"),
		ExchangeType: cfg.GetString("amqp.exchange.type"),
	}

	// Create the messaging client.
	amqpClient, err := createMessagingClient(amqpSettings)
	if err != nil {
		e.Logger.Fatalf("unable to create the messaging client: %s", err.Error())
	}

	// Establish the database connection.
	e.Logger.Info("establishing the database connection")
	databaseURI := cfg.GetString("notifications.db.uri")
	db, err := db.InitDatabase("postgres", databaseURI)
	if err != nil {
		e.Logger.Fatalf("service initialization failed: %s", err.Error())
	}

	// Build the outbound email processor, absorbed from the retired de-mailer service. Both
	// the /mail endpoint and the email_requests consumer below drive it.
	fromAddress := cfg.GetString("email.fromAddress")

	// The relay settings are all optional; their defaults describe the unauthenticated
	// cleartext relay this service talked to before they existed.
	cfg.SetDefault("email.smtpPort", 25)
	smtpDialer, err := mailer.NewDialer(mailer.SMTPSettings{
		Host:               cfg.GetString("email.smtpHost"),
		Port:               cfg.GetInt("email.smtpPort"),
		User:               cfg.GetString("email.smtpUser"),
		Password:           cfg.GetString("email.smtpPassword"),
		LocalName:          cfg.GetString("email.smtpLocalName"),
		CACertFile:         cfg.GetString("email.smtpCACertFile"),
		UseTLS:             cfg.GetBool("email.smtpUseTLS"),
		UseSSL:             cfg.GetBool("email.smtpUseSSL"),
		InsecureSkipVerify: cfg.GetBool("email.smtpInsecureSkipVerify"),
	})
	if err != nil {
		e.Logger.Fatalf("invalid SMTP configuration: %s", err.Error())
	}

	emailProcessor := mailer.NewEmailProcessor(
		mailer.NewEmailClient(smtpDialer, fromAddress),
		mailer.DESettings{
			Base:        cfg.GetString("de.base"),
			Data:        cfg.GetString("de.data"),
			Analyses:    cfg.GetString("de.analyses"),
			Teams:       cfg.GetString("de.teams"),
			Tools:       cfg.GetString("de.tools"),
			Collections: cfg.GetString("de.collections"),
			Apps:        cfg.GetString("de.apps"),
			Admin:       cfg.GetString("de.admin"),
			DOI:         cfg.GetString("de.doi"),
			VICE:        cfg.GetString("de.vice"),
		},
		fromAddress,
	)

	// Callers send bare usernames; the DE stores them qualified.
	userSuffix := common.NewUserSuffix(cfg.GetString("notifications.uid.domain"))

	// Define the primary API handler.
	a := api.API{
		Echo:         e,
		AMQPSettings: amqpSettings,
		AMQPClient:   amqpClient,
		DB:           db,
		UserSuffix:   userSuffix,
		Mailer:       emailProcessor,
		Service:      serviceName,
		Title:        serviceInfo.Title,
		Version:      serviceInfo.Version,
	}

	// Register the handlers.
	a.RegisterHandlers()

	// Record the notification events that the v1 API publishes. The consumer gets its own
	// connection because the messaging client dedicates a connection to listening, and the recorder
	// gets a third one so that a failed publish on its behalf doesn't reconnect the connection the
	// API publishes on. Both are closed by the ordered shutdown at the end of main rather than
	// by defers, so that they outlive the mailer drain.
	e.Logger.Info("starting the event recorder")
	consumerClient, err := messaging.NewClient(amqpSettings.URI, true)
	if err != nil {
		e.Logger.Fatalf("unable to create the consumer messaging client: %s", err.Error())
	}

	recorderClient, err := createMessagingClient(amqpSettings)
	if err != nil {
		e.Logger.Fatalf("unable to create the recorder messaging client: %s", err.Error())
	}

	consumer := recorder.NewConsumer(
		consumerClient,
		recorderClient,
		amqpSettings,
		cfg.GetString("email.request"),
		recorder.New(recorder.NewDatabaseClient(db), recorderClient, userSuffix),
	)
	if err = consumer.Listen(); err != nil {
		e.Logger.Fatalf("unable to start recording notification events: %s", err.Error())
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Send the email requests the recorder publishes. This gets a fourth connection rather
	// than sharing the recorder's: shutdown drains and closes it on its own, which would stop
	// the recorder too if they shared one. It starts in the background so a broker outage
	// can't keep the HTTP transport from coming up; mailer.StartConsumer retries until the
	// broker is reachable.
	e.Logger.Info("starting the email request consumer")
	mailerReady := make(chan *mailer.Consumer, 1)
	go func() {
		// The result is always sent, nil included, so that shutdown can tell "it never started"
		// from "it hasn't reported yet" instead of racing this send and skipping the drain.
		mailerReady <- mailer.StartConsumer(signalCtx, emailProcessor, amqpSettings)
	}()

	// Start the service.
	e.Logger.Info("starting the service")
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- e.Start(fmt.Sprintf(":%d", optionValues.Port))
	}()

	select {
	case err := <-serverErr:
		e.Logger.Fatal(err)
	case <-signalCtx.Done():
	}

	// Ordered shutdown: quiesce every producer of AMQP traffic before any connection closes.
	// In-flight AMQP handlers finish and ack, so already-sent emails aren't requeued and re-sent,
	// and in-flight HTTP requests finish publishing rather than failing on a closed connection.
	// The drains run concurrently because they're independent and the total has to fit in the
	// pod's termination grace period.
	e.Logger.Info("shutting down")
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		var mailConsumer *mailer.Consumer
		select {
		case mailConsumer = <-mailerReady:
		case <-time.After(mailerReadyTimeout):
			e.Logger.Warn("the email request consumer did not report that it started; skipping its drain")
			return
		}
		if mailConsumer != nil {
			mailConsumer.Shutdown(drainTimeout)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		consumer.Drain(drainTimeout)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancelShutdown()
		if err := e.Shutdown(shutdownCtx); err != nil {
			e.Logger.Errorf("HTTP server shutdown: %s", err.Error())
		}
	}()

	wg.Wait()

	consumerClient.Close()
	recorderClient.Close()
	amqpClient.Close()
}
