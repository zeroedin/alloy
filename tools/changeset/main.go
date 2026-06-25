package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		if err := runVersion(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runCreate()
}

// runCreate interactively creates a new changeset file.
func runCreate() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Select bump type:")
	fmt.Println("  1) patch")
	fmt.Println("  2) minor")
	fmt.Println("  3) major")
	fmt.Print("> ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var bumpType string
	switch choice {
	case "1", "patch":
		bumpType = "patch"
	case "2", "minor":
		bumpType = "minor"
	case "3", "major":
		bumpType = "major"
	default:
		fmt.Fprintf(os.Stderr, "invalid bump type: %s\n", choice)
		os.Exit(1)
	}

	fmt.Print("Summary: ")
	summary, _ := reader.ReadString('\n')
	summary = strings.TrimSpace(summary)
	if summary == "" {
		fmt.Fprintln(os.Stderr, "summary cannot be empty")
		os.Exit(1)
	}

	if err := os.MkdirAll(".changeset", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create .changeset directory: %v\n", err)
		os.Exit(1)
	}

	slug := slugify(summary)
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf(".changeset/%s-%s.md", timestamp, slug)

	content := fmt.Sprintf("---\ntype: %s\n---\n\n%s\n", bumpType, summary)

	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write changeset: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created %s\n", filename)
}

type changeset struct {
	bumpType string
	summary  string
}

var ignoredFiles = map[string]bool{
	"readme.md": true,
	"agents.md": true,
	"claude.md": true,
	"gemini.md": true,
}

// readChangesets finds and parses all changeset files in .changeset/.
func readChangesets() ([]changeset, []string, error) {
	entries, err := os.ReadDir(".changeset")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading .changeset: %w", err)
	}

	var changesets []changeset
	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".md") {
			continue
		}
		if ignoredFiles[strings.ToLower(name)] {
			continue
		}

		path := filepath.Join(".changeset", name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s: %w", path, err)
		}

		cs, err := parseChangeset(string(data))
		if err != nil {
			return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		changesets = append(changesets, cs)
		files = append(files, path)
	}

	return changesets, files, nil
}

// parseChangeset extracts the bump type and summary from a changeset file.
func parseChangeset(content string) (changeset, error) {
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return changeset{}, fmt.Errorf("missing frontmatter delimiters")
	}

	frontmatter := strings.TrimSpace(parts[1])
	summary := strings.TrimSpace(parts[2])

	var bumpType string
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "type:") {
			bumpType = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
			break
		}
	}

	switch bumpType {
	case "major", "minor", "patch":
	default:
		return changeset{}, fmt.Errorf("invalid bump type: %q", bumpType)
	}

	return changeset{bumpType: bumpType, summary: summary}, nil
}

func buildDescriptions(changesets []changeset) string {
	levels := make(map[string]bool)
	for _, cs := range changesets {
		levels[cs.bumpType] = true
	}

	var b strings.Builder

	if len(levels) <= 1 {
		for _, cs := range changesets {
			b.WriteString(cs.summary)
			b.WriteString("\n")
		}
		return b.String()
	}

	order := []struct {
		level  string
		header string
	}{
		{"major", "### Major Changes"},
		{"minor", "### Minor Changes"},
		{"patch", "### Patch Changes"},
	}

	first := true
	for _, o := range order {
		if !levels[o.level] {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		b.WriteString(o.header)
		b.WriteString("\n\n")
		for _, cs := range changesets {
			if cs.bumpType == o.level {
				b.WriteString(cs.summary)
				b.WriteString("\n")
			}
		}
		first = false
	}

	return b.String()
}

var bumpPriority = map[string]int{
	"patch": 1,
	"minor": 2,
	"major": 3,
}

// highestBump returns the highest bump type across all changesets.
func highestBump(changesets []changeset) string {
	best := "patch"
	for _, cs := range changesets {
		if bumpPriority[cs.bumpType] > bumpPriority[best] {
			best = cs.bumpType
		}
	}
	return best
}

var versionRegex = regexp.MustCompile(`Version\s*=\s*"(\d+\.\d+\.\d+)"`)

// readVersion reads the current version from cmd/version.go.
func readVersion() (string, error) {
	data, err := os.ReadFile("cmd/version.go")
	if err != nil {
		return "", fmt.Errorf("reading cmd/version.go: %w", err)
	}

	matches := versionRegex.FindSubmatch(data)
	if matches == nil {
		return "", fmt.Errorf("no version found in cmd/version.go")
	}

	return string(matches[1]), nil
}

// writeVersion updates the version in cmd/version.go.
func writeVersion(oldVersion, newVersion string) error {
	data, err := os.ReadFile("cmd/version.go")
	if err != nil {
		return fmt.Errorf("reading cmd/version.go: %w", err)
	}

	updated := strings.Replace(
		string(data),
		fmt.Sprintf(`Version = "%s"`, oldVersion),
		fmt.Sprintf(`Version = "%s"`, newVersion),
		1,
	)

	return os.WriteFile("cmd/version.go", []byte(updated), 0o644)
}

// bumpVersion calculates the next version.
func bumpVersion(current, bump string) (string, error) {
	parts := strings.Split(current, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid version: %s", current)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch version: %w", err)
	}

	switch bump {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}

// writeChangelog prepends a new version entry to CHANGELOG.md.
func writeChangelog(version, descriptions string) error {
	date := time.Now().Format("2006-01-02")

	var entry strings.Builder
	fmt.Fprintf(&entry, "## v%s (%s)\n\n", version, date)
	entry.WriteString(descriptions)

	existing, err := os.ReadFile("CHANGELOG.md")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading CHANGELOG.md: %w", err)
	}

	var final strings.Builder
	final.WriteString(entry.String())
	if len(existing) > 0 {
		final.WriteString("\n")
		final.Write(existing)
	}

	return os.WriteFile("CHANGELOG.md", []byte(final.String()), 0o644)
}

// runVersion processes pending changesets: bumps version, writes changelog,
// updates cmd/version.go, and deletes consumed changeset files.
// Outputs the new version and release notes path for the workflow.
func runVersion() error {
	changesets, files, err := readChangesets()
	if err != nil {
		return err
	}

	if len(changesets) == 0 {
		fmt.Println("No pending changesets found.")
		return nil
	}

	bump := highestBump(changesets)

	currentVersion, err := readVersion()
	if err != nil {
		return err
	}

	newVersion, err := bumpVersion(currentVersion, bump)
	if err != nil {
		return err
	}

	fmt.Printf("Bump: %s | %s -> %s\n", bump, currentVersion, newVersion)

	descriptions := buildDescriptions(changesets)

	// Write CHANGELOG.md
	if err := writeChangelog(newVersion, descriptions); err != nil {
		return err
	}

	// Update version in cmd/version.go
	if err := writeVersion(currentVersion, newVersion); err != nil {
		return err
	}

	// Delete consumed changeset files
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("deleting %s: %w", f, err)
		}
	}

	// Write outputs for the workflow
	if err := os.WriteFile("/tmp/release-version.txt", []byte(newVersion), 0o644); err != nil {
		return fmt.Errorf("writing version output: %w", err)
	}
	if err := os.WriteFile("/tmp/release-notes.md", []byte(descriptions), 0o644); err != nil {
		return fmt.Errorf("writing release notes output: %w", err)
	}

	fmt.Printf("Updated CHANGELOG.md and cmd/version.go to v%s\n", newVersion)
	return nil
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case !prevDash && b.Len() > 0:
			b.WriteByte('-')
			prevDash = true
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}
