package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Prompt prints msg and reads a single line from stdin (without the newline).
func Prompt(r *bufio.Reader, msg string) (string, error) {
	fmt.Fprint(os.Stderr, msg)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// PromptYesNo asks a Y/n style question. Defaults to yes on empty input.
func PromptYesNo(r *bufio.Reader, msg string) (bool, error) {
	ans, err := Prompt(r, msg)
	if err != nil {
		return false, err
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans == "" || ans == "y" || ans == "yes" {
		return true, nil
	}
	return false, nil
}

// PromptMultiline reads lines from stdin until EOF or an empty line. The empty
// line itself is not included.
func PromptMultiline(r *bufio.Reader, msg string) (string, error) {
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	var sb strings.Builder
	for {
		line, err := r.ReadString('\n')
		trim := strings.TrimRight(line, "\r\n")
		if trim == "" && (err == nil || err == io.EOF) {
			if err == io.EOF && sb.Len() == 0 && line == "" {
				return "", nil
			}
			break
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(trim)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

// NewSessionID generates a session ID like "sess_20260501_143012_a1b2".
func NewSessionID() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to time-only suffix.
		return fmt.Sprintf("sess_%s_0000", time.Now().Format("20060102_150405"))
	}
	return fmt.Sprintf("sess_%s_%s", time.Now().Format("20060102_150405"), hex.EncodeToString(b[:]))
}

// Stdin returns a buffered reader on os.Stdin.
func Stdin() *bufio.Reader { return bufio.NewReader(os.Stdin) }
