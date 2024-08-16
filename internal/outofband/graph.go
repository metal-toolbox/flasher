package outofband

import (
	"context"

	"github.com/emicklei/dot"
	"github.com/metal-toolbox/flasher/internal/device"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/metal-toolbox/flasher/internal/runner"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"

	bconsts "github.com/bmc-toolbox/bmclib/v2/constants"
	rctypes "github.com/metal-toolbox/rivets/condition"
	rtypes "github.com/metal-toolbox/rivets/types"
)

func GraphSteps(ctx context.Context, g *dot.Graph) error {
	// setup a mock device queryor
	m := new(device.MockOutofbandQueryor)
	m.On("FirmwareInstallSteps", mock.Anything, "drive").Once().Return(
		[]bconsts.FirmwareInstallStep{
			bconsts.FirmwareInstallStepUploadInitiateInstall,
			bconsts.FirmwareInstallStepInstallStatus,
		},
		nil,
	)

	testActionCtx := &runner.ActionHandlerContext{
		TaskHandlerContext: &runner.TaskHandlerContext{
			Task: &model.Task{
				Parameters: &rctypes.FirmwareInstallTaskParameters{
					ResetBMCBeforeInstall: true,
				},
				Server: &rtypes.Server{},
			},
			Logger:        logrus.NewEntry(logrus.New()),
			DeviceQueryor: m,
		},
		Firmware: &model.Firmware{
			Version:   "DL6R",
			URL:       "https://downloads.dell.com/FOLDER06303849M/1/Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			FileName:  "Serial-ATA_Firmware_Y1P10_WN32_DL6R_A00.EXE",
			Models:    []string{"r6515"},
			Checksum:  "4189d3cb123a781d09a4f568bb686b23c6d8e6b82038eba8222b91c380a25281",
			Component: "drive",
		},
	}

	oh := ActionHandler{}
	action, err := oh.ComposeAction(ctx, testActionCtx)
	if err != nil {
		return err
	}

	for idx, step := range action.Steps {
		cNode := g.Node(string(step.Name))

		var pNode dot.Node
		if idx == 0 {
			pNode = g.Node("Run")
		} else {
			pNode = g.Node(string(action.Steps[idx-1].Name))
		}

		g.Edge(pNode, cNode, step.Description)
		g.Edge(cNode, g.Node(string(model.StateFailed)), "Task Failed")
	}

	lastNode := g.Node(string(action.Steps[len(action.Steps)-1].Name))
	g.Edge(lastNode, g.Node(string(model.StateSucceeded)), "Task Successful")

	return nil
}
