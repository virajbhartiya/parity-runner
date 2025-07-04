package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theblitlabs/deviceid"

	"github.com/theblitlabs/parity-runner/internal/core/models"
)

type HTTPTaskClient struct {
	baseURL string
}

func NewHTTPTaskClient(baseURL string) *HTTPTaskClient {
	return &HTTPTaskClient{
		baseURL: baseURL,
	}
}

func (c *HTTPTaskClient) FetchTask() (*models.Task, error) {
	tasks, err := c.GetAvailableTasks()
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks available")
	}

	task := tasks[0]
	if err := c.StartTask(task.ID.String()); err != nil {
		return nil, err
	}

	return task, nil
}

func (c *HTTPTaskClient) UpdateTaskStatus(taskID string, status models.TaskStatus, result *models.TaskResult) error {
	switch status {
	case models.TaskStatusRunning:
		return c.StartTask(taskID)
	case models.TaskStatusCompleted, models.TaskStatusFailed:
		if err := c.CompleteTask(taskID); err != nil {
			return err
		}
		if result != nil {
			return c.SaveTaskResult(taskID, result)
		}
		return nil
	default:
		return fmt.Errorf("unsupported status: %s", status)
	}
}

func (c *HTTPTaskClient) GetAvailableTasks() ([]*models.Task, error) {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/runners/tasks/available", baseURL)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var tasks []*models.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return tasks, nil
}

func (c *HTTPTaskClient) StartTask(taskID string) error {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/runners/tasks/%s/start", baseURL, taskID)

	deviceIDManager := deviceid.NewManager(deviceid.Config{})
	deviceID, err := deviceIDManager.VerifyDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Device-ID", deviceID)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP POST failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusConflict:
		return fmt.Errorf("task unavailable: %s", string(body))
	case http.StatusBadRequest:
		return fmt.Errorf("bad request: %s", string(body))
	case http.StatusNotFound:
		return fmt.Errorf("task not found")
	default:
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}
}

func (c *HTTPTaskClient) CompleteTask(taskID string) error {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/runners/tasks/%s/complete", baseURL, taskID)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("HTTP POST failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPTaskClient) SaveTaskResult(taskID string, result *models.TaskResult) error {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/runners/tasks/%s/result", baseURL, taskID)

	deviceIDManager := deviceid.NewManager(deviceid.Config{})
	deviceID, err := deviceIDManager.VerifyDeviceID()
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	if result.TaskID == uuid.Nil {
		result.TaskID = uuid.MustParse(taskID)
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now()
	}
	if result.RunnerAddress == "" {
		result.RunnerAddress = deviceID
	}

	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", deviceID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP POST failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPTaskClient) CompletePrompt(promptID uuid.UUID, response string, promptTokens, responseTokens int, inferenceTime int64) error {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/llm/prompts/%s/complete", baseURL, promptID.String())

	payload := map[string]interface{}{
		"response":          response,
		"prompt_tokens":     promptTokens,
		"response_tokens":   responseTokens,
		"inference_time_ms": inferenceTime,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal completion payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP POST failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// SubmitFLModelUpdate submits federated learning model updates to the server
func (c *HTTPTaskClient) SubmitFLModelUpdate(sessionID, roundID, runnerID string, gradients map[string][]float64, weights map[string][]float64, dataSize int, loss, accuracy float64, trainingTime int) error {
	baseURL := strings.TrimSuffix(c.baseURL, "/api")
	url := fmt.Sprintf("%s/api/v1/federated-learning/model-updates", baseURL)

	payload := map[string]interface{}{
		"session_id":    sessionID,
		"round_id":      roundID,
		"runner_id":     runnerID,
		"gradients":     gradients,
		"weights":       weights,
		"update_type":   "gradients",
		"data_size":     dataSize,
		"loss":          loss,
		"accuracy":      accuracy,
		"training_time": trainingTime,
		"metadata": map[string]interface{}{
			"submission_time": time.Now().Unix(),
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal FL model update: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create FL model update request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second, // Longer timeout for FL operations
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP POST failed for FL model update %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("FL model update server error: %s", errResp.Error)
		}
		return fmt.Errorf("FL model update unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
