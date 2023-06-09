package types

import (
	"encoding/json"
	"time"
)

const (
	Version int32 = 1
)

// StatusValue is the canonical structure
type StatusValue struct {
	UpdatedAt time.Time       `json:"updated"`
	WorkerID  string          `json:"worker"`
	Target    string          `json:"target"`
	State     string          `json:"state"`
	Status    json.RawMessage `json:"status"`
	Version   int32           `json:"version"`
	// WorkSpec json.RawMessage `json:"spec"` XXX: for re-publish use-cases
}

// MustBytes sets the version field of the StatusValue so any callers don't have
// to deal with it. It will panic if we cannot serialize to JSON for some reason.
func (v *StatusValue) MustBytes() []byte {
	v.Version = Version
	byt, err := json.Marshal(v)
	if err != nil {
		panic("unable to serialize status value: " + err.Error())
	}
	return byt
}
