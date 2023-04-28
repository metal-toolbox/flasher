package fixtures

import (
	"net"

	"github.com/metal-toolbox/flasher/internal/model"

	"github.com/google/uuid"
)

var (
	Asset1ID = uuid.New()
	Asset2ID = uuid.New()

	Assets = map[string]model.Asset{
		Asset1ID.String(): {
			ID:          Asset1ID,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.1"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},

		Asset2ID.String(): {
			ID:          Asset2ID,
			Vendor:      "dell",
			Model:       "r6515",
			BmcAddress:  net.ParseIP("127.0.0.2"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},
	}
)
