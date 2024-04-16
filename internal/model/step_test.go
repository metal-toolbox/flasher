package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepsByName(t *testing.T) {
	steps := Steps{
		&Step{Name: "initialize", Group: "group1"},
		&Step{Name: "configure", Group: "group2"},
	}

	tests := []struct {
		name        string
		stepName    StepName
		expected    Step
		expectError bool
	}{
		{"Found", "initialize", *steps[0], false},
		{"Not Found", "install", Step{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := steps.ByName(tt.stepName)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStepsByGroup(t *testing.T) {
	steps := Steps{
		&Step{Name: "initialize", Group: "group1"},
		&Step{Name: "configure", Group: "group2"},
		&Step{Name: "execute", Group: "group1"},
	}

	tests := []struct {
		name        string
		groupName   StepGroup
		expected    Steps
		expectError bool
	}{
		{"Group Found", "group1", Steps{steps[0], steps[2]}, false},
		{"Group Not Found", "group3", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := steps.ByGroup(tt.groupName)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStepsRemove(t *testing.T) {
	steps := Steps{
		&Step{Name: "initialize", Group: "group1"},
		&Step{Name: "configure", Group: "group2"},
		&Step{Name: "execute", Group: "group1"},
	}

	tests := []struct {
		name     string
		stepName StepName
		expected Steps
	}{
		{"Remove Exist", "initialize", Steps{steps[1], steps[2]}},
		{"Remove Non-Exist", "shutdown", Steps{steps[0], steps[1], steps[2]}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := steps.Remove(tt.stepName)
			assert.Equal(t, tt.expected, result)
		})
	}
}
