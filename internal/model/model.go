package model

type (
	AppKind   string
	StoreKind string
	// LogLevel is the logging level string.
	LogLevel string
)

const (
	AppName               = "flasher"
	AppKindWorker AppKind = "worker"
	AppKindCLI    AppKind = "cli"

	InventoryStoreYAML          StoreKind = "yaml"
	InventoryStoreServerservice StoreKind = "serverservice"

	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelTrace LogLevel = "trace"
)

// AppKinds returns the supported flasher app kinds
func AppKinds() []AppKind { return []AppKind{AppKindWorker} }

// StoreKinds returns the supported asset inventory, firmware configuration sources
func StoreKinds() []StoreKind {
	return []StoreKind{InventoryStoreYAML, InventoryStoreServerservice}
}
