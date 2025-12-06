package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
)

type ToolConfig map[string]string

type CopyStats struct {
	Success      int
	SkippedDups  int
	Errors       int
	ErrorDetails []string
}

func main() {
	copyFlag := pflag.BoolP("copy", "c", false, "Copy tools from locations in tools.json to GarudReconBinary directory and create zip")
	pasteFlag := pflag.BoolP("paste", "p", false, "Unzip GarudReconBinary.zip and move files to locations specified in tools.json")
	configFlag := pflag.StringP("config", "", "", "Custom path to tools.json file (default: ~/.config/GarudReconBinary/tools.json)")
	pflag.Parse()

	if !*copyFlag && !*pasteFlag {
		fmt.Fprintf(os.Stderr, "Error: Please specify either -c/--copy or -p/--paste flag\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s -c, --copy   Copy tools and create zip\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -p, --paste  Unzip and move tools to destinations\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --config     Custom path to tools.json\n", os.Args[0])
		os.Exit(1)
	}

	if *copyFlag && *pasteFlag {
		fmt.Fprintf(os.Stderr, "Error: Cannot use both -c/--copy and -p/--paste flags at the same time\n")
		os.Exit(1)
	}

	// Determine config file path
	configPath := *configFlag
	if configPath == "" {
		var err error
		configPath, err = getDefaultConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting default config path: %v\n", err)
			os.Exit(1)
		}
	}

	// Download tools.json if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("tools.json not found at %s\n", configPath)
		fmt.Println("Downloading tools.json from GitHub...")
		if err := downloadToolsConfig(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading tools.json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully downloaded tools.json to %s\n", configPath)
	}

	if *copyFlag {
		runCopy(configPath)
	} else if *pasteFlag {
		runPaste(configPath)
	}
}

// runCopy handles the --copy flag functionality
func runCopy(configPath string) {
	// Read tools.json
	config, err := readToolsConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading tools.json: %v\n", err)
		os.Exit(1)
	}

	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Create GarudReconBinary subdirectory if it doesn't exist
	destDir := filepath.Join(workDir, "GarudReconBinary")
	err = os.MkdirAll(destDir, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	stats := &CopyStats{
		ErrorDetails: make([]string, 0),
	}

	// Track seen keys to handle duplicates
	seenKeys := make(map[string]bool)

	fmt.Println("Starting tool copy process...")
	fmt.Printf("Destination directory: %s\n\n", destDir)

	// Process each tool entry
	for toolName, sourcePath := range config {
		// Skip duplicates (keep first occurrence)
		if seenKeys[toolName] {
			fmt.Printf("[SKIP] Duplicate key '%s' - keeping first occurrence\n", toolName)
			stats.SkippedDups++
			continue
		}
		seenKeys[toolName] = true

		// Extract original filename from source path
		originalFilename := filepath.Base(sourcePath)

		// Skip if filename is empty
		if originalFilename == "" || originalFilename == "." {
			fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: invalid filename", toolName))
			continue
		}

		// Destination path
		destPath := filepath.Join(destDir, originalFilename)

		// Copy the file
		err := copyFile(sourcePath, destPath)
		if err != nil {
			// Check if it's a "file not found" error
			if os.IsNotExist(err) {
				fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			} else {
				fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			}
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: %v", toolName, err))
			continue
		}

		fmt.Printf("[OK] Copied '%s' -> %s\n", toolName, originalFilename)
		stats.Success++
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Copy Summary:")
	fmt.Printf("  Successfully copied: %d\n", stats.Success)
	fmt.Printf("  Skipped duplicates: %d\n", stats.SkippedDups)
	fmt.Printf("  Errors: %d\n", stats.Errors)

	if len(stats.ErrorDetails) > 0 {
		fmt.Println("\nError Details:")
		for _, detail := range stats.ErrorDetails {
			fmt.Printf("  - %s\n", detail)
		}
	}
	fmt.Println(strings.Repeat("=", 60))

	// Create zip file of GarudReconBinary directory
	fmt.Println("\nCreating zip archive...")
	zipPath := filepath.Join(workDir, "GarudReconBinary.zip")
	err = createZip(destDir, zipPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating zip file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully created: %s\n", zipPath)

	// Delete GarudReconBinary directory to save space
	fmt.Println("\nDeleting GarudReconBinary directory...")
	err = os.RemoveAll(destDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to delete directory %s: %v\n", destDir, err)
	} else {
		fmt.Printf("Successfully deleted: %s\n", destDir)
	}
}

// runPaste handles the --paste flag functionality
func runPaste(configPath string) {
	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Check if zip file exists
	zipPath := filepath.Join(workDir, "GarudReconBinary.zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: GarudReconBinary.zip not found in current directory\n")
		os.Exit(1)
	}

	// Read tools.json
	config, err := readToolsConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading tools.json: %v\n", err)
		os.Exit(1)
	}

	// Create temporary directory for extraction
	tempDir := filepath.Join(workDir, "temp_garud_extract")
	defer os.RemoveAll(tempDir) // Clean up temp directory

	fmt.Println("Unzipping GarudReconBinary.zip...")
	err = unzipFile(zipPath, tempDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error unzipping file: %v\n", err)
		os.Exit(1)
	}

	// Find the extracted GarudReconBinary directory
	extractedDir := filepath.Join(tempDir, "GarudReconBinary")
	if _, err := os.Stat(extractedDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: GarudReconBinary directory not found in zip\n")
		os.Exit(1)
	}

	stats := &CopyStats{
		ErrorDetails: make([]string, 0),
	}

	// Track seen keys to handle duplicates
	seenKeys := make(map[string]bool)

	fmt.Println("\nStarting tool paste process...")
	fmt.Println("Moving files to destination locations...")

	// Process each tool entry
	for toolName, destPath := range config {
		// Skip duplicates (keep first occurrence)
		if seenKeys[toolName] {
			fmt.Printf("[SKIP] Duplicate key '%s' - keeping first occurrence\n", toolName)
			stats.SkippedDups++
			continue
		}
		seenKeys[toolName] = true

		// Extract original filename from destination path
		originalFilename := filepath.Base(destPath)

		// Skip if filename is empty
		if originalFilename == "" || originalFilename == "." {
			fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: invalid filename", toolName))
			continue
		}

		// Source path in extracted directory
		sourcePath := filepath.Join(extractedDir, originalFilename)

		// Check if source file exists
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: file not found in zip", toolName))
			continue
		}

		// Create destination directory if it doesn't exist
		destDir := filepath.Dir(destPath)
		err = os.MkdirAll(destDir, 0755)
		if err != nil {
			fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: failed to create destination directory: %v", toolName, err))
			continue
		}

		// Move the file to destination
		err = moveFile(sourcePath, destPath)
		if err != nil {
			fmt.Printf("[ERROR] %s -> MISSING\n", toolName)
			stats.Errors++
			stats.ErrorDetails = append(stats.ErrorDetails, fmt.Sprintf("%s: %v", toolName, err))
			continue
		}

		fmt.Printf("[OK] Moved '%s' -> %s\n", toolName, destPath)
		stats.Success++
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Paste Summary:")
	fmt.Printf("  Successfully moved: %d\n", stats.Success)
	fmt.Printf("  Skipped duplicates: %d\n", stats.SkippedDups)
	fmt.Printf("  Errors: %d\n", stats.Errors)

	if len(stats.ErrorDetails) > 0 {
		fmt.Println("\nError Details:")
		for _, detail := range stats.ErrorDetails {
			fmt.Printf("  - %s\n", detail)
		}
	}
	fmt.Println(strings.Repeat("=", 60))

	// Set executable permissions for directories extracted from tools.json
	fmt.Println("\nSetting executable permissions...")
	dirsToChmod := extractDirectoriesFromConfig(config)

	for _, dir := range dirsToChmod {
		err := setExecutablePermissions(dir)
		if err != nil {
			fmt.Printf("Warning: Failed to set permissions for %s: %v\n", dir, err)
		} else {
			fmt.Printf("Set executable permissions for: %s/*\n", dir)
		}
	}
}

// readToolsConfig reads and parses the tools.json file
func readToolsConfig(filename string) (ToolConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer file.Close()

	var config ToolConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return config, nil
}

// copyFile copies a file from source to destination, preserving permissions
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	// Check if source is a regular file
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}

	// Create destination file (will overwrite if exists)
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy file contents
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Preserve file permissions
	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// createZip creates a zip file containing all files from the source directory
func createZip(sourceDir, zipPath string) error {
	// Create the zip file
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	// Create a new zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the source directory and add files to zip
	err = filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path from source directory
		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// Create zip entry with directory structure preserved
		zipEntryPath := filepath.Join("GarudReconBinary", relPath)
		zipEntry, err := zipWriter.Create(zipEntryPath)
		if err != nil {
			return fmt.Errorf("failed to create zip entry: %w", err)
		}

		// Open source file
		srcFile, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open source file: %w", err)
		}
		defer srcFile.Close()

		// Copy file contents to zip entry
		_, err = io.Copy(zipEntry, srcFile)
		if err != nil {
			return fmt.Errorf("failed to copy file to zip: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	return nil
}

// unzipFile extracts a zip file to the destination directory
func unzipFile(zipPath, destDir string) error {
	// Open zip file
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer zipReader.Close()

	// Create destination directory
	err = os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Extract all files
	for _, file := range zipReader.File {
		// Get file path
		filePath := filepath.Join(destDir, file.Name)

		// Skip directories
		if file.FileInfo().IsDir() {
			err = os.MkdirAll(filePath, file.FileInfo().Mode())
			if err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create parent directories
		err = os.MkdirAll(filepath.Dir(filePath), 0755)
		if err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Open file from zip
		zipFile, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		// Create destination file
		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			zipFile.Close()
			return fmt.Errorf("failed to create destination file: %w", err)
		}

		// Copy file contents
		_, err = io.Copy(destFile, zipFile)
		destFile.Close()
		zipFile.Close()

		if err != nil {
			return fmt.Errorf("failed to copy file contents: %w", err)
		}
	}

	return nil
}

// moveFile moves a file from source to destination, preserving permissions
func moveFile(src, dst string) error {
	// Get source file info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	// Copy file first
	err = copyFile(src, dst)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Remove source file after successful copy
	err = os.Remove(src)
	if err != nil {
		// If removal fails, try to remove destination to maintain consistency
		os.Remove(dst)
		return fmt.Errorf("failed to remove source file: %w", err)
	}

	// Preserve file permissions
	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// setExecutablePermissions sets executable permissions for all files in a directory
func setExecutablePermissions(dirPath string) error {
	// Check if directory exists
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, skip silently
		}
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	if !dirInfo.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dirPath)
	}

	// Read directory contents
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Set executable permission for each file
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		filePath := filepath.Join(dirPath, entry.Name())

		// Get current file info
		fileInfo, err := entry.Info()
		if err != nil {
			continue // Skip if we can't get info
		}

		// Set executable permission (add 0111 to current mode)
		newMode := fileInfo.Mode() | 0111
		err = os.Chmod(filePath, newMode)
		if err != nil {
			return fmt.Errorf("failed to chmod %s: %w", filePath, err)
		}
	}

	return nil
}

// extractDirectoriesFromConfig extracts unique directory paths from the tools.json config
func extractDirectoriesFromConfig(config ToolConfig) []string {
	dirMap := make(map[string]bool)
	dirs := []string{}

	// Extract unique directories from all tool paths
	for _, toolPath := range config {
		dir := filepath.Dir(toolPath)
		// Only add if not already in map
		if !dirMap[dir] {
			dirMap[dir] = true
			dirs = append(dirs, dir)
		}
	}

	return dirs
}

// getDefaultConfigPath returns the default path for tools.json
// ~/.config/GarudReconBinary/tools.json
func getDefaultConfigPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	configDir := filepath.Join(usr.HomeDir, ".config", "GarudReconBinary")
	configPath := filepath.Join(configDir, "tools.json")

	return configPath, nil
}

// downloadToolsConfig downloads tools.json from GitHub and saves it to the specified path
func downloadToolsConfig(configPath string) error {
	// Create directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Download from GitHub
	url := "https://raw.githubusercontent.com/rix4uni/GarudReconBinary/refs/heads/main/tools.json"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download tools.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download tools.json: HTTP %d", resp.StatusCode)
	}

	// Create the file
	outFile, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer outFile.Close()

	// Write the body to file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
