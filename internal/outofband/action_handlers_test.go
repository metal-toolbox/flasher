package outofband

//func Test_conditionPowerOnDevice(t *testing.T) {
//
//	tctx := newtaskHandlerContextFixture(tc.task.ID.String(), &model.Device{})
//
//	ctx := context.Background()
//	// init new state machine
//	m, err := NewActionStateMachine(ctx, "testing")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	assert.Equal(t, transitionOrder(), m.TransitionOrder())
//	assert.Len(t, transitionRules(), 9)
//
//	// TODO(joel): at some point we'd want to test if the nodes and edges in the transition rules
//	// match whats expected
//	// run transition
//	//	err = m.Run(ctx, &fixtureAction, tctx)
//	//	if err != nil {
//	//		if !tc.expectError {
//	//			t.Fatal(err)
//	//		}
//	//	}
//
//	// TODO: spawn http service to return dummy firmware file
//	// assert file is uploaded and firmware install is initiated
//	// :)
//
//	// lookup task from cache
//	//task, _ := tctx.Store.TaskByID(ctx, tc.task.ID.String())
//
//	//spew.Dump(fixtureAction)
//
//}
