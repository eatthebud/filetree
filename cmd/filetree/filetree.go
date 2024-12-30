package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	colorReset      = "\033[0m"
	colorPink       = "\033[38;5;205m"
	colorGreen      = "\033[32m"
	colorLightGreen = "\033[38;5;118m"
	colorYellow     = "\033[33m"
	colorTeal       = "\033[38;5;51m"
)

func loadGitignore(path string) ([]string, error) {
	var patterns []string

	file, err := os.Open(path)
	if err != nil {
		// Return an empty slice if .gitignore doesn't exist
		if os.IsNotExist(err) {
			return patterns, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return patterns, nil
}

func loadIgnorePatterns(dir string) ([]string, error) {
	var allPatterns []string

	// Load .gitignore patterns
	gitignorePath := filepath.Join(dir, ".gitignore")
	gitPatterns, err := loadGitignore(gitignorePath)
	if err != nil {
		return nil, fmt.Errorf("error loading .gitignore: %v", err)
	}
	allPatterns = append(allPatterns, gitPatterns...)

	// Load .filetree.toml patterns
	filetreeIgnorePath := filepath.Join(dir, ".filetree.toml")
	filetreePatterns, err := loadGitignore(filetreeIgnorePath)
	if err != nil {
		return nil, fmt.Errorf("error loading .filetree.toml: %v", err)
	}
	allPatterns = append(allPatterns, filetreePatterns...)

	return allPatterns, nil
}

func matchesGitignore(path string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
		// Check for directory patterns like "folder/" or "folder"
		if (strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, strings.TrimSuffix(pattern, "/"))) ||
			(filepath.Base(path) == pattern) {
			return true
		}
	}
	return false
}

type authorStat struct {
	email      string
	count      int
	percentage float64
}

func getFileContributions(path string) (map[string]int, int, error) {
	cmd := fmt.Sprintf("git blame --line-porcelain %s | grep \"^author-mail\" | cut -d \"<\" -f2 | cut -d \">\" -f1", path)
	output, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return nil, 0, err
	}

	authorCounts := make(map[string]int)
	totalLines := 0
	for _, author := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if author != "" {
			authorCounts[author]++
			totalLines++
		}
	}
	return authorCounts, totalLines, nil
}

func calculateAndSortStats(authorCounts map[string]int, totalLines int) []authorStat {
	var stats []authorStat
	if totalLines > 0 {
		for email, count := range authorCounts {
			percentage := float64(count) / float64(totalLines) * 100
			stats = append(stats, authorStat{
				email:      email,
				count:      count,
				percentage: percentage,
			})
		}

		// Sort by count in descending order
		sort.Slice(stats, func(i, j int) bool {
			return stats[i].count > stats[j].count
		})
	}
	return stats
}

func getPercentageColor(percentage float64) string {
	switch {
	case percentage > 75:
		return colorPink
	case percentage > 60:
		return colorGreen
	case percentage > 50:
		return colorLightGreen
	case percentage > 25:
		return colorYellow
	case percentage > 0:
		return colorTeal
	default:
		return colorReset
	}
}

func printDirectories(path string, prefix string, patterns []string, showFiles bool) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fileInfo.IsDir() || matchesGitignore(path, patterns) {
		return nil
	}

	// Print the current directory
	fmt.Println(prefix + "├── " + fileInfo.Name())

	// Read directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	// For directory-level stats when showFiles is false
	dirAuthorCounts := make(map[string]int)
	dirTotalLines := 0

	for i, entry := range entries {
		newPath := filepath.Join(path, entry.Name())

		if matchesGitignore(newPath, patterns) {
			continue
		}

		// Adjust the prefix for the last entry
		newPrefix := prefix + "│   "
		if i == len(entries)-1 {
			newPrefix = prefix + "    "
		}

		if entry.IsDir() {
			if err := printDirectories(newPath, newPrefix, patterns, showFiles); err != nil {
				return err
			}
		} else {
			authorCounts, totalLines, err := getFileContributions(newPath)
			if err != nil {
				return err
			}

			if showFiles {
				stats := calculateAndSortStats(authorCounts, totalLines)
				if len(stats) > 0 {
					fmt.Println(newPrefix + "├── " + entry.Name())
					for _, stat := range stats {
						color := getPercentageColor(stat.percentage)
						fmt.Printf("%s│   ├── %s (%s%.1f%%%s)\n", newPrefix, stat.email, color, stat.percentage, colorReset)
					}
				}
			} else {
				// Aggregate stats for directory level
				for author, count := range authorCounts {
					dirAuthorCounts[author] += count
					dirTotalLines += count
				}
			}
		}
	}

	// Print directory-level stats if we're not showing files
	if !showFiles && dirTotalLines > 0 {
		stats := calculateAndSortStats(dirAuthorCounts, dirTotalLines)
		for _, stat := range stats {
			color := getPercentageColor(stat.percentage)
			fmt.Printf("%s│   ├── %s (%s%.1f%%%s)\n", prefix, stat.email, color, stat.percentage, colorReset)
		}
	}

	return nil
}

func main() {
	// Parse command line flags
	var showFiles bool
	flag.BoolVar(&showFiles, "files", false, "Show files in directory tree")
	flag.BoolVar(&showFiles, "f", false, "Show files in directory tree (shorthand)")
	flag.Parse()

	// Get current directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		return
	}

	// Load ignore patterns from both .gitignore and .filetree.toml
	patterns, err := loadIgnorePatterns(dir)
	if err != nil {
		fmt.Printf("Error loading ignore patterns: %v\n", err)
		return
	}

	// Print the directory tree
	if err := printDirectories(dir, "", patterns, showFiles); err != nil {
		fmt.Printf("Error printing directory tree: %v\n", err)
		return
	}
}
