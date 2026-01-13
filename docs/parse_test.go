package docs

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateOkxApiIndex(t *testing.T) {
	// Read the content of docs/okx.md
	filePath := "okx.md"
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %s", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Failed to read file: %s", err)
	}

	// Regular expressions
	reRequest := regexp.MustCompile(`^#### (HTTP请求|服务地址)`)
	reTitle := regexp.MustCompile(`^### (.*)`)
	reNextTitle := regexp.MustCompile(`^### `)
	// Use a double-quoted string for the regex to avoid issues with backticks.
	reUrl := regexp.MustCompile("`((GET|POST|DELETE|PUT) /api/v5/[^`]+)`")

	type ApiEndpoint struct {
		Title     string
		URL       string
		StartLine int
		EndLine   int
	}

	var endpoints []ApiEndpoint

	for i, line := range lines {
		if reRequest.MatchString(line) {
			var endpoint ApiEndpoint
			endpoint.StartLine = i + 1

			// Find URL
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				urlMatches := reUrl.FindStringSubmatch(lines[j])
				if len(urlMatches) > 1 {
					endpoint.URL = strings.TrimSpace(urlMatches[1])
					break
				}
				trimmedLine := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmedLine, "GET /api/v5/") || strings.HasPrefix(trimmedLine, "POST /api/v5/") {
					endpoint.URL = trimmedLine
					break
				}
			}

			// Find Title
			for j := i - 1; j >= 0; j-- {
				titleMatches := reTitle.FindStringSubmatch(lines[j])
				if len(titleMatches) > 1 {
					endpoint.Title = strings.TrimSpace(titleMatches[1])
					break
				}
			}

			// Find EndLine
			endpoint.EndLine = len(lines)
			for j := i + 1; j < len(lines); j++ {
				if reNextTitle.MatchString(lines[j]) {
					endpoint.EndLine = j
					break
				}
			}

			if endpoint.Title != "" && endpoint.URL != "" {
				endpoints = append(endpoints, endpoint)
			}
		}
	}

	// Write the index file
	outputFile, err := os.Create("okx_api_index.md")
	if err != nil {
		t.Fatalf("Failed to create output file: %s", err)
	}
	defer outputFile.Close()

	writer := bufio.NewWriter(outputFile)
	for _, endpoint := range endpoints {
		line := fmt.Sprintf("line: %d-%d, %s  %s\n", endpoint.StartLine, endpoint.EndLine, endpoint.Title, endpoint.URL)
		_, err := writer.WriteString(line)
		if err != nil {
			t.Fatalf("Failed to write to output file: %s", err)
		}
	}
	writer.Flush()

	t.Logf("Successfully generated okx_api_index.md with %d endpoints.", len(endpoints))
}
