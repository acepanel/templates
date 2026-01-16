package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-resty/resty/v2"
	"go.yaml.in/yaml/v4"
)

const (
	apiURL    = "https://api.acepanel.net/template/import"
	batchSize = 10
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Println("Error: API_KEY environment variable is not set")
		os.Exit(1)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Find all data.yml files
	var templates []map[string]any
	entries, err := os.ReadDir(cwd)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dataPath := filepath.Join(cwd, entry.Name(), "data.yml")
		if _, err := os.Stat(dataPath); os.IsNotExist(err) {
			continue
		}

		// Read and parse data.yml
		data, err := os.ReadFile(dataPath)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", dataPath, err)
			continue
		}

		var template map[string]any
		if err := yaml.Unmarshal(data, &template); err != nil {
			fmt.Printf("Error parsing %s: %v\n", dataPath, err)
			continue
		}

		// Add directory name as template slug
		template["slug"] = entry.Name()

		// Convert environments from map to array format
		// From: {ENV_NAME: {description: xxx, type: xxx, default: xxx}}
		// To: [{name: ENV_NAME, description: xxx, type: xxx, default: xxx}]
		if envMap, ok := template["environments"].(map[string]any); ok {
			envArray := make([]map[string]any, 0, len(envMap))
			for name, value := range envMap {
				if envValue, ok := value.(map[string]any); ok {
					envItem := map[string]any{"name": name}
					for k, v := range envValue {
						envItem[k] = v
					}
					envArray = append(envArray, envItem)
				}
			}
			template["environments"] = envArray
		}

		// Read docker-compose.yml
		composePath := filepath.Join(cwd, entry.Name(), "docker-compose.yml")
		composeData, err := os.ReadFile(composePath)
		if err != nil {
			fmt.Printf("Error reading docker-compose.yml for %s: %v\n", entry.Name(), err)
			continue
		}
		template["compose"] = string(composeData)

		// Read and encode logo (check svg first, then png)
		var logoPath string
		var logoMime string
		svgPath := filepath.Join(cwd, entry.Name(), "logo.svg")
		pngPath := filepath.Join(cwd, entry.Name(), "logo.png")

		if _, err := os.Stat(svgPath); err == nil {
			logoPath = svgPath
			logoMime = "image/svg+xml"
		} else if _, err := os.Stat(pngPath); err == nil {
			logoPath = pngPath
			logoMime = "image/png"
		}

		if logoPath != "" {
			logoData, err := os.ReadFile(logoPath)
			if err != nil {
				fmt.Printf("Error reading logo for %s: %v\n", entry.Name(), err)
			} else {
				template["icon"] = "data:" + logoMime + ";base64," + base64.StdEncoding.EncodeToString(logoData)
			}
		}

		templates = append(templates, template)
		fmt.Printf("Loaded template: %s\n", entry.Name())
	}

	fmt.Printf("\nTotal templates loaded: %d\n", len(templates))

	if len(templates) == 0 {
		fmt.Println("No templates found")
		return
	}

	// Send templates in batches
	client := resty.New()
	totalBatches := (len(templates) + batchSize - 1) / batchSize

	for i := 0; i < len(templates); i += batchSize {
		end := i + batchSize
		if end > len(templates) {
			end = len(templates)
		}

		batch := templates[i:end]
		batchNum := i/batchSize + 1

		jsonData, err := json.Marshal(batch)
		if err != nil {
			fmt.Printf("Error marshaling batch %d: %v\n", batchNum, err)
			continue
		}

		fmt.Printf("\nSending batch %d/%d (%d templates)...\n", batchNum, totalBatches, len(batch))

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("X-API-KEY", apiKey).
			SetBody(jsonData).
			Post(apiURL)

		if err != nil {
			fmt.Printf("Error sending batch %d: %v\n", batchNum, err)
			continue
		}

		if resp.IsSuccess() {
			fmt.Printf("Batch %d sent successfully (status: %d)\n", batchNum, resp.StatusCode())
		} else {
			fmt.Printf("Batch %d failed (status: %d): %s\n", batchNum, resp.StatusCode(), resp.String())
		}
	}

	fmt.Println("\nImport completed!")
}
