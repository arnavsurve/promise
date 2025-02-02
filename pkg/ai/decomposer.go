package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/arnavsurve/promise/pkg/models"
	"github.com/google/uuid"
)

func LLMDecompositionQuery(taskDescription string) ([]models.TaskResponse, error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	url := "https://api.groq.com/openai/v1/chat/completions"

	prompt := fmt.Sprintf(`You are a task decomposition engine that outputs JSON. Break down the following task into an array of JSON formatted subtasks that individual AI agents can accomplish and integrate into a final solution:

        Task: "%s"

        Expected JSON output format:
        [
            {
                "subtask_id": 1
	            "description": "Subtask description",
	            "type": "code_generation",
	            "dependencies": []
	        }
            {
                "subtask_id": 2
	            "description": "Subtask description",
	            "type": "code_generation",
	            "dependencies": [1]
	        }
            {
                "subtask_id": 3
	            "description": "Subtask description",
	            "type": "command_execution",
	            "dependencies": [1, 2]
	        }
            {
                "subtask_id": 4
	            "description": "Subtask description",
	            "type": "command_execution",
	            "dependencies": [1, 2, 3]
	        }
	    ]

        A task can be decomposed into as many subtasks as necessary. 
        Keep in mind context and details of subtask execution will be passed onto dependents of the subtask. Do not implement this here. Only return the expected JSON formatted fields.
        If the task can be accomplished in one shell command, only output one subtask describing what needs to be done.

        Note that a subtask can only be of type:
            command_execution (a task involving shell commands to be run),
            code_generation (generating code),
            prose_generation (generating, editing, or combining plaintext NOT CODE). 

        If a task can be accomplished in a single Bash command, simply create one command_execution subtask to execute this.
        Code generation will create a file automatically and handoff the file location and execution instructions to the next dependent task. Do not create a subtask for saving generated code to a file.

        Dependencies are determined by which subtasks are required to be complete before work begins on the dependent subtask.
        Assign this based on what best fits the subtask.

	    Only output valid JSON as per the expected output denoted above.`, taskDescription)

	requestData := RequestPayload{
		Model: "llama-3.3-70b-versatile",
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var groqResponse GroqResponse
	err = json.Unmarshal(body, &groqResponse)
	if err != nil {
		return nil, err
	}

	if len(groqResponse.Choices) == 0 {
		return nil, fmt.Errorf("LLM response was unsuccessful")
	}

	subtasksJSONString := groqResponse.Choices[0].Message.Content

	// Parse response JSON string into TaskResponse struct
	var taskResponse struct {
		Subtasks []struct {
			SubtaskId    int    `json:"subtask_id"`
			Description  string `json:"description"`
			Type         string `json:"type"`
			Dependencies []int  `json:"dependencies"`
		} `json:"subtasks"`
	}

	err = json.Unmarshal([]byte(subtasksJSONString), &taskResponse)
	if err != nil {
		return nil, err
	}

	taskId := uuid.New()
	var tasks []models.TaskResponse

	for _, subtask := range taskResponse.Subtasks {
		// Convert []int dependencies to []Dependency
		var dependencies []models.Dependency
		for _, depId := range subtask.Dependencies {
			dependencies = append(dependencies, models.Dependency{
				TaskId:    taskId, // Share the same task_id generated above
				SubtaskId: depId,
			})
		}

		// Append task AFTER processing dependencies
		tasks = append(tasks, models.TaskResponse{
			TaskId:       taskId,
			SubtaskId:    subtask.SubtaskId,
			Type:         subtask.Type,
			Description:  subtask.Description,
			Dependencies: dependencies,
			Status:       "pending",
		})
	}

	return tasks, nil
}
