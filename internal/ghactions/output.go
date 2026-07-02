package ghactions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// RenderOutputs formats ordered pairs for the $GITHUB_OUTPUT file. Single-line values use the
// `key=value` form; a value containing a newline uses GitHub's heredoc form with a
// collision-proof delimiter, so a multiline (or attacker-influenced) value can't inject extra
// outputs by smuggling `\n other=...` lines. A key carrying `\n`, `\r`, `=`, or `<` is how output
// injection is attempted, so it's rejected loudly.
func RenderOutputs(outputs []Pair) (string, error) {
	var b strings.Builder
	for _, p := range outputs {
		if err := validateKey(p.Key); err != nil {
			return "", err
		}
		if !strings.ContainsAny(p.Value, "\r\n") {
			b.WriteString(p.Key + "=" + p.Value + "\n")
			continue
		}
		delim := heredocDelimiter(p.Value)
		b.WriteString(p.Key + "<<" + delim + "\n" + p.Value + "\n" + delim + "\n")
	}
	return b.String(), nil
}

func validateKey(key string) error {
	if key == "" {
		return errors.New("output key is empty")
	}
	if strings.ContainsAny(key, "\r\n=<") {
		return fmt.Errorf("invalid output key %q: must not contain newline, '=', or '<'", key)
	}
	return nil
}

// heredocDelimiter returns a delimiter guaranteed not to appear as a line of value: it embeds a
// hash of value, extended until absent in the (astronomically unlikely) collision case.
func heredocDelimiter(value string) string {
	sum := sha256.Sum256([]byte(value))
	delim := "ghadelimiter_" + hex.EncodeToString(sum[:])
	for strings.Contains(value, delim) {
		delim += "_"
	}
	return delim
}

// AppendOutput appends rendered outputs to the $GITHUB_OUTPUT file. The path is a GHA-provided
// sink (global state), not a parameter; it errors if unset so a misconfigured action fails loudly.
func AppendOutput(path string, outputs []Pair) error {
	if path == "" {
		return errors.New("GITHUB_OUTPUT is not set")
	}
	rendered, err := RenderOutputs(outputs)
	if err != nil {
		return err
	}
	return AppendFile(path, rendered)
}

// AppendFile appends raw content to a file — e.g. the $GITHUB_STEP_SUMMARY markdown sink.
func AppendFile(path, content string) error {
	if path == "" {
		return errors.New("file path is empty")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
