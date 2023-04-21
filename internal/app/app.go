package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
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

	// Sync waitgroup to wait for running go routines on termination.
	SyncWG *sync.WaitGroup
	// Flasher configuration.
	Config *Configuration
	// TermCh is the channel to terminate the app based on a signal
	TermCh chan os.Signal
	// Logger is the app logger
	Logger *logrus.Logger
	// Kind is the type of application - worker
	Kind model.AppKind
}

// New returns returns a new instance of the flasher app
func New(ctx context.Context, appKind model.AppKind, storeKind model.StoreKind, cfgFile, loglevel string) (*App, error) {
	if appKind != model.AppKindWorker {
		return nil, errors.Wrap(ErrAppInit, "invalid app kind: "+string(appKind))
	}

	app := &App{
		v:      viper.New(),
		Kind:   appKind,
		SyncWG: &sync.WaitGroup{},
		Logger: logrus.New(),
		TermCh: make(chan os.Signal),
	}

	if err := app.LoadConfiguration(cfgFile, storeKind); err != nil {
		return nil, err
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
	signal.Notify(app.TermCh, syscall.SIGINT, syscall.SIGTERM)

	return app, nil
}
