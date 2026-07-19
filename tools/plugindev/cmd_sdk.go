package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const sdkDirName = "plugindev/sdk"

// sdkStore returns the root directory for stored SDK versions.
func sdkStore() string {
	if v := os.Getenv("HOMEAGENT_SDK_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".homeagent", sdkDirName)
}

func sdkCurrentDir() string {
	return filepath.Join(sdkStore(), "current")
}

func sdkVersionDir(version string) string {
	return filepath.Join(sdkStore(), version)
}

const sdkRepoURL = "https://gitcode.com/JianFeeeee/homeagent-sdk.git"

func cmdSDK(args []string) {
	if len(args) < 1 {
		sdkHelp()
		return
	}
	switch args[0] {
	case "list":
		cmdSDKList()
	case "install":
		if len(args) < 2 {
			fmt.Println("Usage: plugindev sdk install <version>")
			os.Exit(1)
		}
		cmdSDKInstall(args[1])
	case "use":
		if len(args) < 2 {
			fmt.Println("Usage: plugindev sdk use <version>")
			os.Exit(1)
		}
		cmdSDKUse(args[1])
	case "path":
		cmdSDKPath()
	case "current":
		cmdSDKCurrent()
	case "latest":
		cmdSDKLatest()
	default:
		sdkHelp()
	}
}

func sdkHelp() {
	fmt.Print(`Usage: plugindev sdk <command>

Manage installed HomeAgent SDK versions.

Commands:
  list                  List installed SDK versions
  install <version>     Download and install an SDK version (tag or branch)
  use <version>         Switch to the specified SDK version for new projects
  path                  Show the current active SDK directory
  current               Show the current active SDK version
  latest                Show the latest available version from remote

Examples:
  plugindev sdk install v0.7.1
  plugindev sdk install latest
  plugindev sdk use v0.7.1
`)
}

// cmdSDKList lists installed SDK versions.
func cmdSDKList() {
	store := sdkStore()
	entries, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No SDK versions installed.")
			return
		}
		fmt.Printf("error: read SDK store %s: %v\n", store, err)
		os.Exit(1)
	}

	current := resolveCurrentVersion(store)

	var versions []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "current" && !strings.HasPrefix(e.Name(), ".") {
			versions = append(versions, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	if len(versions) == 0 {
		fmt.Println("No SDK versions installed.")
		return
	}
	fmt.Println("Installed SDK versions:")
	for _, v := range versions {
		mark := " "
		if v == current {
			mark = "*"
		}
		fmt.Printf("  %s %s\n", mark, v)
	}
	if current == "" {
		fmt.Println("\nNo version active. Use 'plugindev sdk use <version>' to set one.")
	}
}

// cmdSDKInstall downloads and installs an SDK version.
func cmdSDKInstall(version string) {
	store := sdkStore()
	if err := os.MkdirAll(store, 0755); err != nil {
		fmt.Printf("error: create SDK store %s: %v\n", store, err)
		os.Exit(1)
	}

	if version == "latest" {
		tag, err := fetchLatestTag()
		if err != nil {
			fmt.Printf("error: fetch latest tag: %v\n", err)
			os.Exit(1)
		}
		version = tag
		fmt.Printf("Latest version: %s\n", version)
	}

	dest := sdkVersionDir(version)
	if _, err := os.Stat(dest); err == nil {
		fmt.Printf("SDK version %s already installed at %s\n", version, dest)
		return
	}

	fmt.Printf("Downloading SDK version %s...\n", version)

	tmpDir, err := os.MkdirTemp("", "homeagent-sdk-*")
	if err != nil {
		fmt.Printf("error: create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	cloneArgs := []string{"clone", "--depth", "1", "--branch", version, sdkRepoURL, tmpDir}
	cmd := exec.Command("git", cloneArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("error: failed to clone SDK version %s: %v\n", version, err)
		fmt.Println("Make sure git is installed and the version tag exists.")
		os.Exit(1)
	}

	if err := os.Rename(tmpDir, dest); err != nil {
		fmt.Printf("error: move SDK to store: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("SDK version %s installed at %s\n", version, dest)

	// Auto-switch to newly installed version if no version is currently active
	if resolveCurrentVersion(store) == "" {
		setCurrentVersion(store, version)
	}
}

// cmdSDKUse switches the active SDK version.
func cmdSDKUse(version string) {
	store := sdkStore()
	verDir := sdkVersionDir(version)
	if _, err := os.Stat(verDir); os.IsNotExist(err) {
		fmt.Printf("SDK version %s is not installed.\n", version)
		fmt.Printf("Install it first: plugindev sdk install %s\n", version)
		os.Exit(1)
	}
	setCurrentVersion(store, version)
	fmt.Printf("Active SDK version set to %s\n", version)
}

// cmdSDKPath prints the active SDK directory.
func cmdSDKPath() {
	store := sdkStore()
	current := resolveCurrentVersion(store)
	if current == "" {
		fmt.Println("No active SDK version set.")
		fmt.Println("Use 'plugindev sdk use <version>' to set one.")
		os.Exit(1)
	}
	fmt.Println(sdkVersionDir(current))
}

// cmdSDKCurrent prints the active SDK version.
func cmdSDKCurrent() {
	store := sdkStore()
	current := resolveCurrentVersion(store)
	if current == "" {
		fmt.Println("No active SDK version set.")
		os.Exit(1)
	}
	fmt.Println(current)
}

// cmdSDKLatest fetches the latest tag from the remote SDK repo.
func cmdSDKLatest() {
	tag, err := fetchLatestTag()
	if err != nil {
		fmt.Printf("error: fetch latest tag: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(tag)
}

// resolveCurrentVersion reads the current active version.
func resolveCurrentVersion(store string) string {
	currentFile := filepath.Join(store, "current")
	data, err := os.ReadFile(currentFile)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return ""
	}
	verDir := filepath.Join(store, v)
	if _, err := os.Stat(verDir); os.IsNotExist(err) {
		return ""
	}
	return v
}

// setCurrentVersion writes the version name to the current file.
func setCurrentVersion(store, version string) {
	currentFile := filepath.Join(store, "current")
	if err := os.WriteFile(currentFile, []byte(version+"\n"), 0644); err != nil {
		fmt.Printf("error: write current version: %v\n", err)
		os.Exit(1)
	}
}

// fetchLatestTag uses git ls-remote to find the latest semver tag.
func fetchLatestTag() (string, error) {
	cmd := exec.Command("git", "ls-remote", "--tags", sdkRepoURL)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}

	tags := parseTags(string(out))
	if len(tags) == 0 {
		return "", fmt.Errorf("no tags found in remote repository")
	}
	return tags[len(tags)-1], nil
}

// parseTags extracts semver tags from git ls-remote output and sorts them.
func parseTags(output string) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := parts[1]
		// Only refs/tags/v*, skip refs/tags/v*-^{}
		if !strings.HasPrefix(ref, "refs/tags/v") || strings.HasSuffix(ref, "^{}") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	sort.Slice(tags, func(i, j int) bool {
		return compareSemver(tags[i], tags[j]) < 0
	})
	return tags
}

// compareSemver compares two semver tags (vX.Y.Z). Returns -1, 0, or 1.
func compareSemver(a, b string) int {
	an := parseSemver(a)
	bn := parseSemver(b)
	for i := 0; i < 3; i++ {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseSemver extracts [major, minor, patch] from a vX.Y.Z string.
func parseSemver(tag string) [3]int {
	var v [3]int
	s := strings.TrimPrefix(tag, "v")
	parts := strings.SplitN(s, ".", 3)
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n := 0
		fmt.Sscanf(p, "%d", &n)
		v[i] = n
	}
	return v
}

// activeSDKRoot returns the path to the active SDK root.
// It replaces the old runtime.Caller(0) approach so plugindev can work
// independently of its own build location.
func activeSDKRoot() string {
	store := sdkStore()
	current := resolveCurrentVersion(store)
	if current == "" {
		fmt.Printf("error: no active SDK version set\n")
		fmt.Printf("  Install one: plugindev sdk install latest\n")
		fmt.Printf("  Or set one:  plugindev sdk use <version>\n")
		os.Exit(1)
	}
	root := sdkVersionDir(current)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		fmt.Printf("error: active SDK version %s not found at %s\n", current, root)
		fmt.Printf("  Reinstall: plugindev sdk install %s\n", current)
		os.Exit(1)
	}
	return root
}

// readMetaVersion reads the Version string from the SDK's meta/meta.go.
// If the file is missing or unreadable, returns "0.0.0".
func readMetaVersion(sdkRoot string) string {
	metaPath := filepath.Join(sdkRoot, "meta", "meta.go")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "0.0.0"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Version = ") {
			v := strings.TrimPrefix(line, "Version = ")
			v = strings.Trim(v, `"`)
			return v
		}
	}
	return "0.0.0"
}


