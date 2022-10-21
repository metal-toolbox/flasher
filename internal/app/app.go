package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/sirupsen/logrus"
)

// Config holds configuration data when running mctl
// App holds attributes for the mtl application
type App struct {
	// Sync waitgroup to wait for running go routines on termination.
	SyncWG *sync.WaitGroup
	// Flasher configuration.
	Config *model.Config
	// TermCh is the channel to terminate the app based on a signal
	TermCh chan os.Signal
	// Logger is the app logger
	Logger *logrus.Logger
}

// New returns returns a new instance of the flasher app
func New(ctx context.Context, appKind model.AppKind, inventorySourceKind, cfgFile string, loglevel int) (*App, error) {
	// load configuration
	cfg := &model.Config{
		AppKind:         appKind,
		InventorySource: inventorySourceKind,
	}

	if err := cfg.Load(cfgFile); err != nil {
		return nil, err
	}

	app := &App{
		Config: cfg,
		SyncWG: &sync.WaitGroup{},
		Logger: logrus.New(),
		TermCh: make(chan os.Signal),
	}

	// set log level, format
	switch loglevel {
	case model.LogLevelDebug:
		app.Logger.Level = logrus.DebugLevel
	case model.LogLevelTrace:
		app.Logger.Level = logrus.TraceLevel
	default:
		app.Logger.Level = logrus.InfoLevel
	}

	app.Logger.SetFormatter(
		&runtime.Formatter{ChildFormatter: &logrus.JSONFormatter{}},
	)

	// register for SIGINT, SIGTERM
	signal.Notify(app.TermCh, syscall.SIGINT, syscall.SIGTERM)

	return app, nil
}
