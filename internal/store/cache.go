package store

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/metal-toolbox/flasher/internal/model"
	"github.com/pkg/errors"
)

var (
	ErrNoTasksFound = errors.New("no tasks found")
)

type Cache struct {
	mu *sync.RWMutex

	// tasks is a map of task IDs to tasks
	tasks map[string]model.Task
}

func NewCacheStore() *Cache {
	return &Cache{tasks: map[string]model.Task{}, mu: &sync.RWMutex{}}
}

func (c *Cache) AddTask(ctx context.Context, task model.Task) (uuid.UUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := uuid.New()
	task.CreatedAt = time.Now()

	c.tasks[id.String()] = task

	return id, nil
}

func (c *Cache) UpdateTask(ctx context.Context, task model.Task) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	task.UpdatedAt = time.Now()

	c.tasks[task.ID.String()] = task

	return nil
}

func (c *Cache) TaskByStatus(ctx context.Context, status string) ([]model.Task, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tasks := []model.Task{}

	for _, t := range c.tasks {
		if t.Status == status {
			tasks = append(tasks, t)
		}
	}

	if len(tasks) == 0 {
		return tasks, errors.Wrap(ErrNoTasksFound, "with status "+status)
	}

	return tasks, nil
}

func (c *Cache) TaskByID(ctx context.Context, id string) (model.Task, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.tasks[id], nil
}

func (c *Cache) RemoveTask(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tasks, id)

	return nil
}
