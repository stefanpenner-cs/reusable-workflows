// Package noinlinescripts enforces the "no inline scripts" rule: every action/workflow `run:` must
// be a single external invocation — a bazelisk/go/bash/sh call or a bare script path — never embedded
// shell logic, and `actions/github-script` is banned (it embeds inline JS). It parses the YAML and
// inspects each step's *folded* `run:` value, so flags split across continuation lines for
// readability are validated as one command (shell operators on a later line are still caught).
// Pure + tested; file discovery lives in tools/no-inline-scripts.
package noinlinescripts

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AllowNames are step names exempt from the rule. Empty: shared.yaml bootstraps via
// stefanpenner/checkout-anywhere (a plain `uses:`), so there is no inline exception.
var AllowNames = map[string]bool{}

var (
	exprRe = regexp.MustCompile(`\$\{\{[^}]*\}\}`)
	// plainOps: redirection/sequencing/pipes — literal inside single OR double quotes, so checked
	// on the fully-unquoted view. subOps: command substitution — executes inside double quotes too,
	// so checked on the single-quote-stripped view (double-quoted content retained).
	plainOps = regexp.MustCompile(`&&|\|\||[;|<>]`)
	subOps   = regexp.MustCompile("`|\\$\\(")
	evalRe   = regexp.MustCompile(`^(node|deno|bun)\s+(-e|--eval|-p|--print)\b`)
	// Accepted interpreters: bazelisk/bazel (the Go runtime model) + go (dev/CI tooling), plus bash/sh.
	interpRe = regexp.MustCompile(`^(go|bash|sh|bazelisk|bazel)\s+\S`)
	bareRe   = regexp.MustCompile(`^\S+\.(mjs|cjs|js|sh)$`)
	ghScript = regexp.MustCompile(`^actions/github-script@`)
	flagRe   = regexp.MustCompile(`--\S`) // a long flag (`--name`); the bare `-- ` separator doesn't match
	// `bash -c`/`sh -c` (incl. short-flag clusters like `-euxc`) and `go generate` are inline logic
	// dressed as an interpreter invocation — the payload/directives are opaque to shellOps.
	shellDashCRe = regexp.MustCompile(`^(bash|sh)\b`)
	dashCFlagRe  = regexp.MustCompile(`^-[a-z]*c([a-z]*)$|^--command$`)
	goGenerateRe = regexp.MustCompile(`^go\s+generate\b`)
)

// IsSingleInvocation reports whether value (the fully-folded run: command) is a single external
// invocation with no embedded logic.
func IsSingleInvocation(value string) bool {
	v := strings.TrimSpace(exprRe.ReplaceAllString(value, "X")) // drop ${{ … }} before inspecting
	switch {
	case v == "":
		return false
	// Shell operators are checked on the UNQUOTED view so a metachar inside a quoted flag value
	// (`--template='<div>'`) is treated as data, not logic — while an unquoted `>`/`;`/`|` is caught.
	case plainOps.MatchString(unquoted(v)) || subOps.MatchString(stripSingleQuoted(v)):
		return false
	case evalRe.MatchString(v): // inline eval defeats the rule even without shell operators
		return false
	case isShellDashC(v): // bash -c / sh -c "..." is inline shell
		return false
	case goGenerateRe.MatchString(v): // go generate runs //go:generate directives
		return false
	case interpRe.MatchString(v):
		return true
	default:
		return bareRe.MatchString(v)
	}
}

// isShellDashC reports whether v is a bash/sh invocation carrying a `-c`/`--command` flag.
func isShellDashC(v string) bool {
	if !shellDashCRe.MatchString(v) {
		return false
	}
	for _, tok := range tokenize(v) {
		if dashCFlagRe.MatchString(tok) {
			return true
		}
	}
	return false
}

// unquoted returns v with the contents of single- and double-quoted spans removed, so plain shell
// operators are only detected outside quotes. Quote characters themselves are dropped.
func unquoted(v string) string { return strip(v, true, true) }

// stripSingleQuoted removes only single-quoted spans, keeping double-quoted content — command
// substitution executes inside double quotes, so it must remain visible to subOps.
func stripSingleQuoted(v string) string { return strip(v, true, false) }

func strip(v string, single, double bool) string {
	var b strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			if !single {
				b.WriteByte(c)
			}
		case c == '"' && !inSingle:
			inDouble = !inDouble
			if !double {
				b.WriteByte(c)
			}
		case inSingle && single, inDouble && double:
			// inside a stripped quote span → drop
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Violation is a single guard failure: a 1-based line number and a message.
type Violation struct {
	Line    int
	Message string
}

// InlineErrors parses one YAML document and returns a Violation for every offending step `run:` (or
// banned `actions/github-script` use). allowNames (a set of exempt step names) is injectable for
// testing; pass AllowNames for the default policy.
func InlineErrors(yamlText string, allowNames map[string]bool) []Violation {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return []Violation{{Line: 1, Message: "could not parse YAML: " + err.Error()}}
	}
	lines := strings.Split(yamlText, "\n")
	var out []Violation
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			name := ""
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == "name" && n.Content[i+1].Kind == yaml.ScalarNode {
					name = n.Content[i+1].Value
				}
			}
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, val := n.Content[i], n.Content[i+1]
				switch {
				case key.Value == "uses" && val.Kind == yaml.ScalarNode && ghScript.MatchString(val.Value):
					out = append(out, Violation{val.Line, "actions/github-script embeds inline JS — write a tested external script instead"})
				case key.Value == "run" && val.Kind == yaml.ScalarNode && !allowNames[name]:
					switch {
					case val.Style == yaml.LiteralStyle || val.Style == yaml.FoldedStyle:
						out = append(out, Violation{val.Line, "block scalar run: — move logic into an external script"})
					case strings.TrimSpace(val.Value) == "":
						out = append(out, Violation{val.Line, "empty run: — nothing to invoke"})
					case !IsSingleInvocation(val.Value):
						out = append(out, Violation{val.Line, fmt.Sprintf("inline logic in run: %q — call an external script instead", val.Value)})
					case val.Line-1 < len(lines) && len(flagRe.FindAllString(lines[val.Line-1], -1)) >= 2:
						// readability: many flags crammed onto the run: line → one per continuation line
						out = append(out, Violation{val.Line, "run: has multiple flags on one line — split them one per line for readability"})
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&doc)
	return out
}
