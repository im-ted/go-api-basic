//+build wireinject

package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gilcrest/go-api-basic/app"
	"github.com/gilcrest/go-api-basic/datastore"
	"github.com/gilcrest/go-api-basic/handler"
	"github.com/google/wire"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"go.opencensus.io/trace"
	"gocloud.dev/server"
	"gocloud.dev/server/driver"
	"gocloud.dev/server/health"
	"gocloud.dev/server/health/sqlhealth"
	"gocloud.dev/server/requestlog"
)

// applicationSet is the Wire provider set for the application
var applicationSet = wire.NewSet(
	app.NewApplication,
	newRouter,
	wire.Bind(new(http.Handler), new(*mux.Router)),
	handler.NewAppHandler,
	app.NewLogger,
)

// goCloudServerSet
var goCloudServerSet = wire.NewSet(
	trace.AlwaysSample,
	server.New,
	server.NewDefaultDriver,
	wire.Bind(new(driver.Server), new(*server.DefaultDriver)),
	wire.Bind(new(requestlog.Logger), new(*requestLogger)),
	newRequestLogger,
)

type requestLogger struct {
	logger zerolog.Logger
}

func (rl requestLogger) Log(e *requestlog.Entry) {
	rl.logger.Info().
		Str("received_time", e.ReceivedTime.Format(time.RFC1123)).
		Str("request_method", e.RequestMethod).
		Str("request_url", e.RequestURL).
		Int64("request_header_size", e.RequestHeaderSize).
		Int64("request_body_size", e.RequestBodySize).
		Str("user_agent", e.UserAgent).
		Str("referer", e.Referer).
		Str("protocol", e.Proto).
		Str("remote_ip", e.RemoteIP).
		Str("server_ip", e.ServerIP).
		Int("status", e.Status).
		Int64("response_header_size", e.ResponseHeaderSize).
		Int64("response_body_size", e.ResponseBodySize).
		Int64("latency in millis", e.Latency.Milliseconds()).
		Str("trace_id", e.TraceID.String()).
		Str("span_id", e.SpanID.String()).
		Msg("request received")
}

func newRequestLogger(l zerolog.Logger) *requestLogger {
	return &requestLogger{logger: l}
}

// setupApp is a Wire injector function that sets up the
// application using a PostgreSQL implementation
func setupApp(ctx context.Context, envName app.EnvName, dsName datastore.Name, loglvl zerolog.Level) (*server.Server, func(), error) {
	// This will be filled in by Wire with providers from the provider sets in
	// wire.Build.
	wire.Build(
		wire.InterfaceValue(new(trace.Exporter), trace.Exporter(nil)),
		goCloudServerSet,
		applicationSet,
		appHealthChecks,
		wire.Struct(new(server.Options), "RequestLogger", "HealthChecks", "TraceExporter", "DefaultSamplingPolicy", "Driver"),
		datastore.NewPGDatasourceName,
		datastore.NewDB,
		wire.Bind(new(datastore.Datastorer), new(*datastore.Datastore)),
		datastore.NewDatastore)
	return nil, nil, nil
}

// appHealthChecks returns a health check for the database. This will signal
// to Kubernetes or other orchestrators that the server should not receive
// traffic until the server is able to connect to its database.
func appHealthChecks(n datastore.Name, db *sql.DB) ([]health.Checker, func()) {
	dbCheck := sqlhealth.New(db)
	list := []health.Checker{dbCheck}
	return list, func() {
		dbCheck.Stop()
	}
}
