package ssr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// customElementRe matches opening tags of custom elements (tags containing hyphens).
var customElementRe = regexp.MustCompile(`<([a-z][a-z0-9]*-[a-z0-9-]*)(\s[^>]*)?>`)

// ScanComponents parses HTML and returns unique custom element tag names
// (tags with hyphens). Returns deduplicated tag names only.
func ScanComponents(html string) []string {
	matches := customElementRe.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, match := range matches {
		tag := match[1]
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// RenderPage pipes full page HTML to the command via stdin and returns
// the transformed HTML from stdout. The command string is parsed into
// executable + args via strings.Fields.
func RenderPage(command string, html string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", &exec.Error{Name: command, Err: exec.ErrNotFound}
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(html)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

// RenderPageWithTimeout pipes full page HTML to the command via stdin,
// respecting the context deadline. The process is killed if the context
// expires before completion.
func RenderPageWithTimeout(ctx context.Context, command string, html string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", &exec.Error{Name: command, Err: exec.ErrNotFound}
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(html)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

// StreamRenderer manages a persistent SSR process that handles multiple
// pages via NUL-delimited stdin/stdout messages.
type StreamRenderer struct {
	command  string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stdoutBr *bufio.Reader
}

// NewStreamRenderer starts a persistent process for stream-mode SSR.
// The process stays alive until Close() is called.
func NewStreamRenderer(command string) (*StreamRenderer, error) {
	sr := &StreamRenderer{command: command}
	if err := sr.start(); err != nil {
		return nil, err
	}
	return sr, nil
}

func (sr *StreamRenderer) start() error {
	parts := strings.Fields(sr.command)
	if len(parts) == 0 {
		return &exec.Error{Name: sr.command, Err: exec.ErrNotFound}
	}

	sr.cmd = exec.Command(parts[0], parts[1:]...)

	var err error
	sr.stdin, err = sr.cmd.StdinPipe()
	if err != nil {
		return err
	}
	sr.stdout, err = sr.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	sr.stdoutBr = bufio.NewReader(sr.stdout)

	if err := sr.cmd.Start(); err != nil {
		return err
	}
	return nil
}

// RenderPage sends HTML to the persistent process via stdin (NUL-terminated)
// and reads the response from stdout (NUL-terminated).
func (sr *StreamRenderer) RenderPage(html string) (string, error) {
	// Write HTML + NUL delimiter
	if _, err := io.WriteString(sr.stdin, html+"\x00"); err != nil {
		return "", err
	}

	// Read until NUL delimiter using buffered reader
	data, err := sr.stdoutBr.ReadBytes('\x00')
	if err != nil {
		return "", fmt.Errorf("stream read: %w", err)
	}
	// Strip the trailing NUL delimiter
	if len(data) > 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}
	return string(data), nil
}

// Restart kills the current process and starts a new one.
func (sr *StreamRenderer) Restart() error {
	// Best-effort cleanup of old process
	if sr.stdin != nil {
		sr.stdin.Close()
	}
	if sr.cmd != nil && sr.cmd.Process != nil {
		sr.cmd.Process.Kill()
		sr.cmd.Wait()
	}
	return sr.start()
}

// Close shuts down the persistent process by closing stdin and waiting for exit.
func (sr *StreamRenderer) Close() error {
	if sr.stdin != nil {
		sr.stdin.Close()
	}
	if sr.cmd != nil {
		return sr.cmd.Wait()
	}
	return nil
}

// HashOutput computes a content hash for Phase 2 output comparison.
// If the hash matches the cached hash, SSR can be skipped for that page.
func HashOutput(html string) string {
	h := sha256.Sum256([]byte(html))
	return hex.EncodeToString(h[:])
}
