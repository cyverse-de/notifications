package main

import (
	"context"
	"fmt"
	"os"

	"github.com/DavidGamba/go-getoptions"
	"github.com/cyverse-de/configurate"
	"github.com/cyverse-de/echo-middleware/v2/redoc"
	"github.com/cyverse-de/go-mod/otelutils"
	"github.com/cyverse-de/messaging/v9"
	"github.com/cyverse-de/notifications/api"
	"github.com/cyverse-de/notifications/common"
	"github.com/cyverse-de/notifications/db"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	_ "github.com/lib/pq"
)

const serviceName = "notifications"

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
		fmt.Fprintf(os.Stderr, opt.Help())
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err)
		fmt.Fprintf(os.Stderr, opt.Help(getoptions.HelpSynopsis))
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

	// Define the primary API handler.
	a := api.API{
		Echo:         e,
		AMQPSettings: amqpSettings,
		AMQPClient:   amqpClient,
		DB:           db,
		Service:      serviceName,
		Title:        serviceInfo.Title,
		Version:      serviceInfo.Version,
	}

	// Register the handlers.
	a.RegisterHandlers()

	// Start the service.
	e.Logger.Info("starting the service")
	e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", optionValues.Port)))
}
