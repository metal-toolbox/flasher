package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	ErrAppInit = errors.New("error initializing app")
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
}

// New returns returns a new instance of the flasher app
func New(ctx context.Context, appKind model.AppKind, storeKind model.StoreKind, cfgFile, loglevel string) (*App, <-chan os.Signal, error) {
	if appKind != model.AppKindWorker {
		return nil, nil, errors.Wrap(ErrAppInit, "invalid app kind: "+string(appKind))
	}

	app := &App{
		v:      viper.New(),
		Kind:   appKind,
		Config: &Configuration{},
		Logger: logrus.New(),
	}

	if err := app.LoadConfiguration(cfgFile, storeKind); err != nil {
		return nil, nil, err
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

	termCh := make(chan os.Signal, 1)

	// register for SIGINT, SIGTERM
	signal.Notify(termCh, syscall.SIGINT, syscall.SIGTERM)

	return app, termCh, nil
}
