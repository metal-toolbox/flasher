package types

import (
	"encoding/json"
	"time"
)

const (
	Version int32 = 1
)

// StatusValue is the canonical structure for reporting status of an ongoing firmware install
type StatusValue struct {
	UpdatedAt       time.Time       `json:"updated"`
	WorkerID        string          `json:"worker"`
	Target          string          `json:"target"`
	TraceID         string          `json:"traceID"`
	SpanID          string          `json:"spanID"`
	State           string          `json:"state"`
	Status          json.RawMessage `json:"status"`
	ResourceVersion int64           `json:"resourceVersion"` // for updates to server-service
	MsgVersion      int32           `json:"msgVersion"`
	// WorkSpec json.RawMessage `json:"spec"` XXX: for re-publish use-cases
}

// MustBytes sets the version field of the StatusValue so any callers don't have
// to deal with it. It will panic if we cannot serialize to JSON for some reason.
func (v *StatusValue) MustBytes() []byte {
	v.MsgVersion = Version
	byt, err := json.Marshal(v)
	if err != nil {
		panic("unable to serialize status value: " + err.Error())
	}
	return byt
}
