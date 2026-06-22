package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
)

func main() {
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
