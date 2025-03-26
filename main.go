package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	hackeroneDataURL = "https://raw.githubusercontent.com/arkadiyt/bounty-targets-data/main/data/hackerone_data.json"
	cacheDir         = ".bounty-monitor"
	cacheFile        = "hackerone_previous.json"
	notificationFile = "notifications.txt"
	checkInterval    = 1 * time.Hour
)

// Program represents a HackerOne bug bounty program
type Program struct {
	Handle          string  `json:"handle"`
	Name            string  `json:"name"`
	URL             string  `json:"url"`
	OffersBounties  bool    `json:"offers_bounties"`
	SubmissionState string  `json:"submission_state"`
	Targets         Targets `json:"targets"`
}

// Targets represents the in-scope and out-of-scope targets
type Targets struct {
	InScope    []Scope `json:"in_scope"`
	OutOfScope []Scope `json:"out_of_scope"`
}

// Scope represents a target scope
type Scope struct {
	AssetIdentifier   string `json:"asset_identifier"`
	AssetType         string `json:"asset_type"`
	EligibleForBounty bool   `json:"eligible_for_bounty"`
	Instruction       string `json:"instruction"`
	MaxSeverity       string `json:"max_severity"`
}

func main() {
	// Set up logging
	logFile, err := os.OpenFile("bounty-monitor.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("Starting bounty monitor service")

	// Ensure cache directory exists
	if err := ensureCacheDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cache directory: %v\n", err)
		log.Fatalf("Failed to create cache directory: %v", err)
		os.Exit(1)
	}

	// Run immediately once
	err = runCheck()
	if err != nil {
		log.Printf("Error in initial check: %v", err)
	}

	// Set up ticker for hourly checks
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	fmt.Println("Bounty monitor is running. Press Ctrl+C to stop.")
	fmt.Printf("Will check for updates every %s\n", checkInterval)

	// Keep the application running and perform checks at every tick
	for {
		select {
		case <-ticker.C:
			log.Println("Running scheduled check")
			err := runCheck()
			if err != nil {
				log.Printf("Error in scheduled check: %v", err)
			}
		}
	}
}

// runCheck fetches current data, compares with previous data, and logs changes
func runCheck() error {
	// Fetch current data
	currentData, err := fetchHackeroneData()
	if err != nil {
		return fmt.Errorf("failed to fetch Hackerone data: %v", err)
	}

	// Load previous data
	previousData, err := loadPreviousData()
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No previous data found. This appears to be the first run.")
			log.Println("Saving current data for future comparison.")
			if err := savePreviousData(currentData); err != nil {
				return fmt.Errorf("failed to save current data: %v", err)
			}
			return nil
		}
		return fmt.Errorf("failed to load previous data: %v", err)
	}

	// Compare data and find changes
	changes := findChanges(previousData, currentData)

	// If there are changes, notify
	if len(changes.newPrograms) > 0 || len(changes.newScopes) > 0 {
		notificationMsg := formatChangeNotification(changes)
		fmt.Println(notificationMsg)
		log.Println(notificationMsg)

		// Save notification to file
		notificationPath := filepath.Join(cacheDir, notificationFile)
		file, err := os.OpenFile(notificationPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("Failed to open notification file: %v", err)
		} else {
			defer file.Close()
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			fmt.Fprintf(file, "[%s]\n%s\n\n", timestamp, notificationMsg)
		}
	} else {
		log.Println("No changes detected")
	}

	// Save current data for next run
	if err := savePreviousData(currentData); err != nil {
		return fmt.Errorf("failed to save current data: %v", err)
	}

	return nil
}

// Changes holds all detected changes between runs
type Changes struct {
	newPrograms []Program
	newScopes   map[Program][]Scope
}

// findChanges identifies new programs and new in-scope targets
func findChanges(previous, current []Program) Changes {
	changes := Changes{
		newPrograms: []Program{},
		newScopes:   make(map[Program][]Scope),
	}

	// Create map of previous program handles
	previousPrograms := make(map[string]bool)
	for _, program := range previous {
		previousPrograms[program.Handle] = true
	}

	// Create a map for previous program scopes
	previousScopes := make(map[string]map[string]bool)

	for _, program := range previous {
		// Skip paused or disabled programs
		if program.SubmissionState != "open" {
			continue
		}

		previousScopes[program.Handle] = make(map[string]bool)
		for _, scope := range program.Targets.InScope {
			// We're only interested in URL and WILDCARD asset types
			if isRelevantAssetType(scope.AssetType) {
				key := scope.AssetType + ":" + scope.AssetIdentifier
				previousScopes[program.Handle][key] = true
			}
		}
	}

	// Find new programs and new scopes
	for _, program := range current {
		// Skip paused or disabled programs
		if program.SubmissionState != "open" {
			continue
		}

		// Check if this is a new program
		if !previousPrograms[program.Handle] {
			changes.newPrograms = append(changes.newPrograms, program)
			continue
		}

		// Check for new scopes
		for _, scope := range program.Targets.InScope {
			// Only check URL and WILDCARD asset types
			if isRelevantAssetType(scope.AssetType) {
				key := scope.AssetType + ":" + scope.AssetIdentifier

				// Check if this scope is new
				if prevProgram, exists := previousScopes[program.Handle]; !exists || !prevProgram[key] {
					changes.newScopes[program] = append(changes.newScopes[program], scope)
				}
			}
		}
	}

	return changes
}

// formatChangeNotification creates a human-readable notification message
func formatChangeNotification(changes Changes) string {
	var notification strings.Builder

	// Report new programs
	if len(changes.newPrograms) > 0 {
		notification.WriteString(fmt.Sprintf("New programs found: %d\n\n", len(changes.newPrograms)))

		// Sort programs by name for consistent output
		sort.Slice(changes.newPrograms, func(i, j int) bool {
			return changes.newPrograms[i].Name < changes.newPrograms[j].Name
		})

		for _, program := range changes.newPrograms {
			notification.WriteString(fmt.Sprintf("=== NEW PROGRAM: %s (%s) ===\n", program.Name, program.Handle))
			notification.WriteString(fmt.Sprintf("Program URL: %s\n", program.URL))
			notification.WriteString(fmt.Sprintf("Offers Bounties: %t\n\n", program.OffersBounties))

			// Count URL and WILDCARD targets
			var urlCount, wildcardCount int
			for _, scope := range program.Targets.InScope {
				if scope.AssetType == "URL" {
					urlCount++
				} else if scope.AssetType == "WILDCARD" {
					wildcardCount++
				}
			}

			notification.WriteString(fmt.Sprintf("In-scope targets: %d URLs, %d Wildcards\n\n", urlCount, wildcardCount))
		}
	}

	// Report new scopes
	if len(changes.newScopes) > 0 {
		notification.WriteString(fmt.Sprintf("New scopes found in existing programs: %d\n\n", len(changes.newScopes)))

		// Sort programs by name for consistent output
		var sortedPrograms []Program
		for program := range changes.newScopes {
			sortedPrograms = append(sortedPrograms, program)
		}
		sort.Slice(sortedPrograms, func(i, j int) bool {
			return sortedPrograms[i].Name < sortedPrograms[j].Name
		})

		for _, program := range sortedPrograms {
			scopes := changes.newScopes[program]

			notification.WriteString(fmt.Sprintf("=== %s (%s) ===\n", program.Name, program.Handle))
			notification.WriteString(fmt.Sprintf("Program URL: %s\n", program.URL))
			notification.WriteString(fmt.Sprintf("Offers Bounties: %t\n", program.OffersBounties))

			// Sort scopes for consistent output
			sort.Slice(scopes, func(i, j int) bool {
				return scopes[i].AssetIdentifier < scopes[j].AssetIdentifier
			})

			for _, scope := range scopes {
				eligibility := ""
				if scope.EligibleForBounty {
					eligibility = " (Eligible for bounty)"
				}

				notification.WriteString(fmt.Sprintf("- [%s] %s%s\n", scope.AssetType, scope.AssetIdentifier, eligibility))
				if scope.MaxSeverity != "" {
					notification.WriteString(fmt.Sprintf("  Max Severity: %s\n", scope.MaxSeverity))
				}
			}
			notification.WriteString("\n")
		}
	}

	return notification.String()
}

// fetchHackeroneData downloads and parses the Hackerone data JSON
func fetchHackeroneData() ([]Program, error) {
	log.Println("Fetching data from", hackeroneDataURL)
	resp, err := http.Get(hackeroneDataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := json.Unmarshal(body, &programs); err != nil {
		return nil, err
	}

	log.Printf("Successfully fetched data: %d programs found", len(programs))
	return programs, nil
}

// loadPreviousData loads the cached Hackerone data from the previous run
func loadPreviousData() ([]Program, error) {
	path := filepath.Join(cacheDir, cacheFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := json.Unmarshal(data, &programs); err != nil {
		return nil, err
	}

	log.Printf("Successfully loaded previous data: %d programs", len(programs))
	return programs, nil
}

// savePreviousData saves the current Hackerone data for the next run
func savePreviousData(programs []Program) error {
	path := filepath.Join(cacheDir, cacheFile)
	data, err := json.MarshalIndent(programs, "", "  ")
	if err != nil {
		return err
	}

	log.Printf("Saving current data: %d programs", len(programs))
	return os.WriteFile(path, data, 0644)
}

// ensureCacheDir ensures the cache directory exists
func ensureCacheDir() error {
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		log.Printf("Creating cache directory: %s", cacheDir)
		return os.MkdirAll(cacheDir, 0755)
	}
	return nil
}

// isRelevantAssetType checks if the asset type is URL or WILDCARD
func isRelevantAssetType(assetType string) bool {
	return strings.ToUpper(assetType) == "URL" || strings.ToUpper(assetType) == "WILDCARD"
}
