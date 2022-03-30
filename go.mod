module github.com/cyverse-de/notifications

go 1.14

require (
	github.com/DavidGamba/go-getoptions v0.20.2
	github.com/Masterminds/squirrel v1.4.0
	github.com/cyverse-de/configurate v0.0.0-20200527185205-4e1e92866cee
	github.com/cyverse-de/dbutil v1.0.1
	github.com/cyverse-de/echo-middleware/v2 v2.0.2
	github.com/cyverse-de/messaging/v9 v9.1.1
	github.com/go-playground/validator/v10 v10.10.0
	github.com/labstack/echo/v4 v4.7.2
	github.com/labstack/gommon v0.3.1
	github.com/lib/pq v1.10.4
	github.com/mcnijman/go-emailaddress v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/viper v1.7.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho v0.31.0
	go.opentelemetry.io/otel v1.6.1
	go.opentelemetry.io/otel/exporters/jaeger v1.6.1
	go.opentelemetry.io/otel/sdk v1.6.1
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)
