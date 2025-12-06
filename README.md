# GarudReconBinary

A Golang tool for copying and managing binary tools from various locations. This tool helps you backup your security tools and restore them to their original locations.

## Features

- **Copy Mode (`-c` / `--copy`)**: Copies binary tools from locations specified in `tools.json` to a `GarudReconBinary` directory, creates a zip archive, and cleans up the temporary directory
- **Paste Mode (`-p` / `--paste`)**: Unzips `GarudReconBinary.zip` and moves files back to their original locations specified in `tools.json`
- **Automatic Permission Management**: Sets executable permissions for all files in directories extracted from `tools.json`
- **Duplicate Handling**: Automatically handles duplicate keys in `tools.json` (keeps first occurrence)
- **Error Handling**: Graceful error handling with detailed logging and summary statistics

## Installation

1. Clone or download this repository
2. Ensure you have Go installed (version 1.23+)
3. Install dependencies:
   ```bash
   go mod tidy
   ```
4. Build the tool:
   ```bash
   go build -o GarudReconBinary GarudReconBinary.go
   ```

## Configuration

The tool uses `tools.json` which contains a mapping of tool names to their file paths:

```json
{
    "nuclei": "/root/go/bin/nuclei",
    "subfinder": "/root/go/bin/subfinder",
    "nmap": "/usr/local/bin/nmap",
    ...
}
```

## Usage

### Copy Mode (`-c` / `--copy`)

Copies all tools from their locations in `tools.json` to a `GarudReconBinary` directory, creates a zip file, and deletes the temporary directory.

```bash
./GarudReconBinary -c
# or
./GarudReconBinary --copy
```

**What it does:**
1. Reads `tools.json` to get tool locations
2. Copies each tool to `GarudReconBinary/` directory
3. Creates `GarudReconBinary.zip` with all tools
4. Deletes the `GarudReconBinary/` directory to save space
5. Shows summary statistics (successful copies, skipped duplicates, errors)

**Output:**
- `GarudReconBinary.zip` - Archive containing all copied tools

### Paste Mode (`-p` / `--paste`)

Unzips `GarudReconBinary.zip` and moves files back to their original locations as specified in `tools.json`.

```bash
./GarudReconBinary -p
# or
./GarudReconBinary --paste
```

**What it does:**
1. Checks for `GarudReconBinary.zip` in the current directory
2. Unzips the archive to a temporary directory
3. Reads `tools.json` to get destination paths
4. Moves each file to its destination location
5. Creates destination directories if they don't exist
6. Sets executable permissions for all files in directories found in `tools.json`
7. Shows summary statistics (successful moves, skipped duplicates, errors)

**Note:** The tool automatically extracts unique directories from `tools.json` and sets `chmod +x` permissions for all files in those directories.

## Examples

### Backup your tools
```bash
./GarudReconBinary -c
```

Output:
```
Starting tool copy process...
Destination directory: /path/to/GarudReconBinary

[OK] Copied 'nuclei' -> nuclei
[OK] Copied 'subfinder' -> subfinder
[ERROR] interlace -> MISSING
...

============================================================
Copy Summary:
  Successfully copied: 150
  Skipped duplicates: 2
  Errors: 5

Creating zip archive...
Successfully created: /path/to/GarudReconBinary.zip

Deleting GarudReconBinary directory...
Successfully deleted: /path/to/GarudReconBinary
```

### Restore your tools
```bash
./GarudReconBinary -p
```

Output:
```
Unzipping GarudReconBinary.zip...

Starting tool paste process...
Moving files to destination locations...

[OK] Moved 'nuclei' -> /root/go/bin/nuclei
[OK] Moved 'subfinder' -> /root/go/bin/subfinder
...

============================================================
Paste Summary:
  Successfully moved: 150
  Skipped duplicates: 2
  Errors: 0

Setting executable permissions...
Set executable permissions for: /root/go/bin/*
Set executable permissions for: /root/.cargo/bin/*
Set executable permissions for: /usr/local/bin/*
...
```

## Error Handling

- **Missing files**: Shows `[ERROR] toolname -> MISSING` for files that don't exist
- **Duplicate keys**: Automatically skips duplicates and keeps the first occurrence
- **Permission errors**: Shows warnings but continues processing
- **Summary**: Displays detailed statistics at the end of each operation

## Requirements

- Go 1.23 or higher
- `github.com/spf13/pflag` package (automatically installed with `go mod tidy`)

## File Structure

```
GarudReconBinary/
├── GarudReconBinary.go    # Main source code
├── tools.json              # Tool configuration file
├── go.mod                  # Go module file
├── go.sum                  # Go checksums
└── README.md               # This file
```

## Notes

- The tool preserves original file permissions when copying/moving
- Destination directories are automatically created if they don't exist
- The tool handles duplicate keys in `tools.json` by keeping the first occurrence
- Executable permissions are automatically set for all directories found in `tools.json` after pasting
- Missing source files are logged but don't stop the process

## License

This tool is provided as-is for personal and professional use.

