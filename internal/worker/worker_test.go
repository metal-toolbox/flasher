package worker

import (
	"context"
	"sync"
	"testing"

	"github.com/metal-toolbox/flasher/internal/fixtures"
	"github.com/metal-toolbox/flasher/internal/inventory"
	"github.com/metal-toolbox/flasher/internal/model"
	sm "github.com/metal-toolbox/flasher/internal/statemachine"
	"github.com/metal-toolbox/flasher/internal/store"
	"github.com/stretchr/testify/assert"
)

func initTestWorker() *Worker {
	inv, _ := inventory.NewMockInventory()
	return &Worker{
		concurrency:  1,
		taskMachines: sync.Map{},
		store:        store.NewMemStore(),
		inv:          inv,
	}
}

func Test_CreateTaskForDevice(t *testing.T) {
	worker := initTestWorker()

	ctx := context.Background()

	err := worker.createTaskForDevice(ctx, fixtures.Devices[fixtures.Device1.String()])
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := worker.store.TasksByStatus(ctx, string(sm.StateQueued))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(tasks))
	assert.Equal(t, fixtures.Devices[fixtures.Device1.String()], tasks[0].Parameters.Device)
	assert.Equal(t, model.PlanFromInstalledFirmware, tasks[0].Parameters.FirmwarePlanMethod)
}
