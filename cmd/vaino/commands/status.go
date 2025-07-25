package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yairfalse/vaino/pkg/config"
)

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show VAINO system and provider status",
		Long: `Display comprehensive status information about VAINO configuration,
providers, authentication, and recent activity.`,
		RunE: runStatus,
	}

	cmd.Flags().BoolP("quiet", "q", false, "show only essential status information")
	cmd.Flags().Bool("short", false, "show brief status summary")

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	quiet, _ := cmd.Flags().GetBool("quiet")
	short, _ := cmd.Flags().GetBool("short")
	homeDir, _ := os.UserHomeDir()

	if !quiet && !short {
		fmt.Println("VAINO Status Report")
		fmt.Println("===================")
		fmt.Println()
	}

	// System Status
	if !short {
		fmt.Println("System Status:")
		fmt.Printf("  VAINO Version: %s\n", getVersion())
	} else {
		fmt.Printf("Version: %s\n", getVersion())
	}

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		configFile = filepath.Join(homeDir, ".vaino", "config.yaml")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			configFile = "not found"
		}
	}
	if !quiet {
		if !short {
			fmt.Printf("  Config File: %s\n", configFile)
			// Storage info
			storagePath := filepath.Join(homeDir, ".vaino")
			storageSize := getDirectorySize(storagePath)
			fmt.Printf("  Storage: %s (%s used)\n", storagePath, formatBytes(storageSize))
			fmt.Println()
			fmt.Println("Provider Status:")
		} else {
			fmt.Println("Providers:")
		}
	}

	detector := config.NewProviderDetector()
	authChecker := config.NewAuthChecker()

	// Terraform status
	terraformResult := detector.DetectTerraform()
	if terraformResult.StateFiles > 0 {
		fmt.Printf("  [OK] Terraform: configured (%d state files found)\n", terraformResult.StateFiles)
	} else {
		fmt.Printf("  [-] Terraform: no state files found\n")
	}

	// GCP status
	gcpResult := detector.DetectGCP()
	if gcpResult.Available {
		gcpAuth := authChecker.CheckGCP()
		if gcpAuth.Authenticated {
			status := "configured"
			if gcpAuth.ProjectID != "" {
				status = fmt.Sprintf("configured (project: %s, authenticated)", gcpAuth.ProjectID)
			}
			fmt.Printf("  [OK] GCP: %s\n", status)
		} else {
			fmt.Printf("  [WARN] GCP: not authenticated\n")
		}
	} else {
		fmt.Printf("  [FAIL] GCP: not configured (gcloud CLI not found)\n")
	}

	// AWS status
	awsResult := detector.DetectAWS()
	if awsResult.Available {
		awsAuth := authChecker.CheckAWS()
		if awsAuth.Authenticated {
			status := "configured"
			if awsAuth.Profile != "" {
				status = fmt.Sprintf("configured (profile: %s", awsAuth.Profile)
				if awsAuth.Region != "" {
					status += fmt.Sprintf(", region: %s", awsAuth.Region)
				}
				status += ")"
			}
			fmt.Printf("  [OK] AWS: %s\n", status)
		} else {
			fmt.Printf("  [WARN] AWS: credentials not found\n")
		}
	} else {
		fmt.Printf("  [FAIL] AWS: not configured (AWS CLI not found)\n")
	}

	// Kubernetes status
	k8sResult := detector.DetectKubernetes()
	if k8sResult.Available {
		k8sAuth := authChecker.CheckKubernetes()
		if k8sAuth.Authenticated {
			status := "configured"
			if k8sAuth.Context != "" {
				status = fmt.Sprintf("configured (context: %s", k8sAuth.Context)
				if k8sAuth.Namespaces > 0 {
					status += fmt.Sprintf(", %d namespaces", k8sAuth.Namespaces)
				}
				status += ")"
			}
			fmt.Printf("  [OK] Kubernetes: %s\n", status)
		} else {
			fmt.Printf("  [WARN] Kubernetes: no cluster access\n")
		}
	} else {
		fmt.Printf("  [FAIL] Kubernetes: not configured (kubectl not found)\n")
	}

	fmt.Println()

	// Recent Activity
	fmt.Println("Recent Activity:")

	lastScanPath := filepath.Join(homeDir, ".vaino", "last-scan-*.json")
	matches, _ := filepath.Glob(lastScanPath)

	if len(matches) > 0 {
		// Find most recent scan
		var mostRecent string
		var mostRecentTime time.Time

		for _, match := range matches {
			info, err := os.Stat(match)
			if err == nil && info.ModTime().After(mostRecentTime) {
				mostRecent = match
				mostRecentTime = info.ModTime()
			}
		}

		if mostRecent != "" {
			// Extract provider from filename
			base := filepath.Base(mostRecent)
			provider := extractProviderFromFilename(base)

			// Calculate time ago
			timeAgo := formatTimeAgo(mostRecentTime)

			// Try to read resource count
			resourceCount := getResourceCount(mostRecent)

			fmt.Printf("  Last Scan: %s ago", timeAgo)
			if provider != "" {
				fmt.Printf(" (%s provider", provider)
				if resourceCount > 0 {
					fmt.Printf(", %d resources found", resourceCount)
				}
				fmt.Printf(")")
			}
			fmt.Println()
		}
	} else {
		fmt.Println("  Last Scan: never")
	}

	// History info
	historyPath := filepath.Join(homeDir, ".vaino", "history")
	historyCount := countFiles(historyPath, "*.json")
	if historyCount > 0 {
		fmt.Printf("  Snapshots: %d stored\n", historyCount)
	}

	fmt.Println()
	fmt.Println("Quick Actions:")
	fmt.Println("  - Run 'vaino scan' to scan infrastructure")
	fmt.Println("  - Run 'vaino configure' to set up providers")
	fmt.Println("  - Run 'vaino check-config' to validate configuration")

	return nil
}

func getVersion() string {
	// This would normally come from build info
	return "1.0.0"
}

func getDirectorySize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%d seconds", int(duration.Seconds()))
	case duration < time.Hour:
		return fmt.Sprintf("%d minutes", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%d hours", int(duration.Hours()))
	case duration < 7*24*time.Hour:
		return fmt.Sprintf("%d days", int(duration.Hours()/24))
	default:
		return fmt.Sprintf("%d weeks", int(duration.Hours()/(24*7)))
	}
}

func extractProviderFromFilename(filename string) string {
	// Format: last-scan-provider.json
	if len(filename) > 14 && filename[:10] == "last-scan-" {
		provider := filename[10:]
		if idx := len(provider) - 5; idx > 0 && provider[idx:] == ".json" {
			return provider[:idx]
		}
	}
	return ""
}

func getResourceCount(filepath string) int {
	// This is a simplified version - in production would properly parse JSON
	data, err := os.ReadFile(filepath)
	if err != nil {
		return 0
	}

	// Simple heuristic: count occurrences of "id" field
	count := 0
	searchStr := `"id":`
	for i := 0; i < len(data)-len(searchStr); i++ {
		if string(data[i:i+len(searchStr)]) == searchStr {
			count++
		}
	}

	return count
}

func countFiles(dir, pattern string) int {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	return len(matches)
}
