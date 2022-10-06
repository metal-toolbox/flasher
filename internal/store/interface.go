package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
)

type Storage interface {
	TasksByStatus(ctx context.Context, status string) ([]model.Task, error)
	TaskByID(ctx context.Context, id string) (model.Task, error)
	AddTask(ctx context.Context, task model.Task) (uuid.UUID, error)
	UpdateTask(ctx context.Context, task model.Task) error
	UpdateTaskAction(ctx context.Context, taskID string, actionName string, action model.Action) error
	RemoveTask(ctx context.Context, id string) error
}
