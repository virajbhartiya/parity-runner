package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type (
	TaskStatus string
	TaskType   string
)

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

const (
	TaskTypeDocker            TaskType = "docker"
	TaskTypeCommand           TaskType = "command"
	TaskTypeLLM               TaskType = "llm"
	TaskTypeFederatedLearning TaskType = "federated_learning"
)

type TaskConfig struct {
	FileURL        string            `json:"file_url,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Resources      ResourceConfig    `json:"resources,omitempty"`
	DockerImageURL string            `json:"docker_image_url,omitempty"`
	ImageName      string            `json:"image_name,omitempty"`
}

func (c *TaskConfig) Validate(taskType TaskType) error {
	switch taskType {
	case TaskTypeDocker:
		if c.ImageName == "" {
			return errors.New("image name is required for Docker tasks")
		}
	case TaskTypeCommand:
	case TaskTypeLLM:
	case TaskTypeFederatedLearning:
	default:
		return fmt.Errorf("unsupported task type: %s", taskType)
	}
	return nil
}

type ResourceConfig struct {
	Memory    string `json:"memory,omitempty"`
	CPUShares int64  `json:"cpu_shares,omitempty"`
	Timeout   string `json:"timeout,omitempty"`
}

type Task struct {
	ID              uuid.UUID          `json:"id" gorm:"type:uuid;primaryKey"`
	Title           string             `json:"title" gorm:"type:varchar(255)"`
	Description     string             `json:"description" gorm:"type:text"`
	Type            TaskType           `json:"type" gorm:"type:varchar(50)"`
	Status          TaskStatus         `json:"status" gorm:"type:varchar(50)"`
	Config          json.RawMessage    `json:"config" gorm:"type:jsonb"`
	Environment     *EnvironmentConfig `json:"environment" gorm:"type:jsonb"`
	Reward          float64            `json:"reward,omitempty" gorm:"type:decimal(20,8)"`
	CreatorAddress  string             `json:"creator_address" gorm:"type:varchar(42)"`
	CreatorDeviceID string             `json:"creator_device_id" gorm:"type:varchar(255)"`
	RunnerID        string             `json:"runner_id" gorm:"type:varchar(255)"`
	Nonce           string             `json:"nonce" gorm:"type:varchar(64);not null"`
	CreatedAt       time.Time          `json:"created_at" gorm:"type:timestamp"`
	UpdatedAt       time.Time          `json:"updated_at" gorm:"type:timestamp"`
	CompletedAt     *time.Time         `json:"completed_at" gorm:"type:timestamp"`
}

func NewTask() *Task {
	return &Task{
		ID:        uuid.New(),
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (t *Task) Validate() error {
	if t.Title == "" {
		return errors.New("title is required")
	}

	if t.Type == "" {
		return errors.New("task type is required")
	}

	var config TaskConfig
	if err := json.Unmarshal(t.Config, &config); err != nil {
		return fmt.Errorf("failed to unmarshal task config: %w", err)
	}

	if err := config.Validate(t.Type); err != nil {
		return err
	}

	if t.Type == TaskTypeDocker && (t.Environment == nil || t.Environment.Type != "docker") {
		return errors.New("docker environment configuration is required for docker tasks")
	}

	return nil
}
