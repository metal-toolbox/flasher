package fixtures

import (
	"net"

	"github.com/metal-toolbox/flasher/internal/model"

	"github.com/google/uuid"
)

var (
	Device1 = uuid.New()
	Device2 = uuid.New()

	Devices = map[string]model.Device{
		Device1.String(): {
			ID:          Device1,
			BmcAddress:  net.ParseIP("127.0.0.1"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},

		Device2.String(): {
			ID:          Device2,
			BmcAddress:  net.ParseIP("127.0.0.2"),
			BmcUsername: "root",
			BmcPassword: "hunter2",
		},
	}
)
