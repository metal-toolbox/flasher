package app

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	// nolint:gosec // pprof path is only exposed over localhost
	_ "net/http/pprof"
)

var (
	ErrAppInit = errors.New("error initializing app")
)

const (
	ProfilingEndpoint = "localhost:9091"
)

// Config holds configuration data when running mctl
// App holds attributes for the mtl application
type App struct {
	// Viper loads configuration parameters.
	v *viper.Viper
	// Flasher configuration.
	Config *Configuration
	// Logger is the app logger
	Logger *logrus.Logger
	// Kind is the type of application - worker
	Kind model.AppKind
	// Mode indicates the means of installing firmware on the target
	Mode model.RunMode
}

// New returns returns a new instance of the flasher app
func New(appKind model.AppKind, storeKind model.StoreKind, cfgFile, loglevel string, profiling bool, mode model.RunMode) (*App, <-chan os.Signal, error) {
	if appKind != model.AppKindWorker && appKind != model.AppKindCLI {
		return nil, nil, errors.Wrap(ErrAppInit, "invalid app kind: "+string(appKind))
	}

	app := &App{
		v:      viper.New(),
		Kind:   appKind,
		Config: &Configuration{},
		Logger: logrus.New(),
		Mode:   mode,
	}

	switch model.LogLevel(loglevel) {
	case model.LogLevelDebug:
		app.Logger.Level = logrus.DebugLevel
	case model.LogLevelTrace:
		app.Logger.Level = logrus.TraceLevel
	default:
		app.Logger.Level = logrus.InfoLevel
	}

	runtimeFormatter := &runtime.Formatter{
		ChildFormatter: &logrus.JSONFormatter{},
		File:           true,
		Line:           true,
		BaseNameOnly:   true,
	}

	app.Logger.SetFormatter(runtimeFormatter)

	// register for SIGINT, SIGTERM
	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh, syscall.SIGINT, syscall.SIGTERM)

	if profiling {
		enableProfilingEndpoint()
	}

	if appKind == model.AppKindCLI {
		runtimeFormatter.ChildFormatter = &logrus.TextFormatter{}
		return app, termCh, nil
	}

	if err := app.LoadConfiguration(cfgFile, storeKind); err != nil {
		return nil, nil, err
	}

	return app, termCh, nil
}

// enableProfilingEndpoint enables the profiling endpoint
func enableProfilingEndpoint() {
	go func() {
		server := &http.Server{
			Addr:              ProfilingEndpoint,
			ReadHeaderTimeout: 2 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	log.Println("profiling enabled: " + ProfilingEndpoint + "/debug/pprof")
}
