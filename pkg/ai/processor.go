package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/arnavsurve/promise/pkg/models"
)

var (
	apiKey = os.Getenv("GROQ_API_KEY")
	url    = "https://api.groq.com/openai/v1/chat/completions"
)

func ProcessTask(task models.Task, depsContext map[string]string) (string, error) {
	switch task.Type {
	case "command_execution":
		return processCommand(task, depsContext)
	case "code_generation":
		return processCodeGeneration(task, depsContext)
	}
	return "", nil
}

// executeCommand runs a shell command and returns the result
func executeCommand(command string, args []string) (string, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %v, output: %s", err, string(output))
	}
	return string(output), err
}

func processCommand(task models.Task, depsContext map[string]string) (string, error) {
	// Construct dependency context from handoff
	depsInfo := ""
	for i, context := range depsContext {
		depsInfo += fmt.Sprintf("%s: %s\n", i, context)
	}

	prompt := fmt.Sprintf(`You are an intelligent command execution agent.
Your task is to convert the following task description into a safe, valid Bash command,
and to produce a "context" documentation that provides key information from this subtask that will be useful for handoff to subsequent agents. When creating the command, be sure to use the absolute path.

Dependency Context from previous workers:
%s

Task: %s

Return a JSON object with the following fields:
  "command": the shell command to run,
  "args": an array of arguments for the command,
  "context": Documentation of the key information and results from this subtask.

Example:
{
  "command": "ls",
  "args": ["-l", "/home/user"],
  "context": "Lists all files with detailed info in /home/user"
}`, depsInfo, task.Description)

	requestData := RequestPayload{
		Model: "deepseek-r1-distill-llama-70b",
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
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var groqResponse GroqResponse
	err = json.Unmarshal(body, &groqResponse)
	if err != nil {
		return "", err
	}

	if len(groqResponse.Choices) == 0 {
		return "", fmt.Errorf("LLM response was unsuccessful")
	}

	var cmdResp CommandResponse
	err = json.Unmarshal([]byte(groqResponse.Choices[0].Message.Content), &cmdResp)
	if err != nil {
		return "", err
	}

	log.Printf("Original command: %s %v", cmdResp.Command, cmdResp.Args)

	// Execute the command
	output, err := executeCommand(cmdResp.Command, cmdResp.Args)
	if err != nil {
		return "", fmt.Errorf("error executing command: %v", err)
	}

	// Combine LLM generated context with command output
	combinedContext := fmt.Sprintf("%s\nCommand Output:\n%s", cmdResp.Context, output)
	log.Printf("Passing context: %s\n", combinedContext)

	return combinedContext, nil
}

func processCodeGeneration(task models.Task, depsContext map[string]string) (string, error) {
	// Construct dependency context from handoff
	depsInfo := ""
	for i, context := range depsContext {
		depsInfo += fmt.Sprintf("%s: %s\n", i, context)
	}

	// Construct the absolute path using the task ID and promise base directory
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %v", err)
	}
	dirPath := fmt.Sprintf("%s/promise/%s/", userHome, task.TaskId)

	prompt := fmt.Sprintf(`You are an intelligent code generation agent.
Your task is to generate executable code based on the following task description.
        The code should be safe and self-contained. Produce a "context" documentation that provides key information from this subtask that will be useful for handoff to subsequent agents. Make sure to refer to any files created using the absolute path: %s when writing the context documentation. When writing instructions, ensure you refer to the file with the name you have selected for it, rather than a placeholder.

Dependency context from previous workers:
%s

Task: %s

Output a JSON object with the following fields:
    "code": a string containing the code to be executed,
    "filename": filename.sh
    "context": Documentation of what the code does and any instructions for execution.

Example:
{
    "code": "#!/bin/bash\necho 'Hello, World!'",
    "filename": "filename.sh",
    "context": "Generates a bash script that prints Hello, World!"
}`, depsInfo, dirPath, task.Description)

	requestData := RequestPayload{
		Model: "deepseek-r1-distill-llama-70b",
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
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var groqResponse GroqResponse
	err = json.Unmarshal(body, &groqResponse)
	if err != nil {
		return "", err
	}

	if len(groqResponse.Choices) == 0 {
		return "", fmt.Errorf("LLM response was unsuccessful")
	}

	var codeResp CodeResponse
	err = json.Unmarshal([]byte(groqResponse.Choices[0].Message.Content), &codeResp)
	if err != nil {
		return "", err
	}

	// Define the directory where code will be saved.
	// We use task.TaskId to create a unique folder per task
	userHome, err = os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %v", err)
	}
	dirPath = fmt.Sprintf("%s/promise/%s/", userHome, task.TaskId)
	if err = os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	// Define the file path for generated code.
	filePath := dirPath + codeResp.Filename
	if err := os.WriteFile(filePath, []byte(codeResp.Code), 0755); err != nil {
		return "", fmt.Errorf("failed to write code to file: %v", err)
	}

	// Combine the location information with the LLM-generated context.
	combinedContext := fmt.Sprintf("Code generated and saved to %s.\n%s", filePath, codeResp.Context)
	log.Printf("Passing context: %s\n", combinedContext)

	return combinedContext, nil
}
