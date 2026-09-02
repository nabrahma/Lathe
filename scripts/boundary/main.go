// Command boundary enforces the two architectural import rules from the design:
// Wails may only be imported by internal/app, and network-capable packages may
// only be imported by internal/deps and internal/update.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	wailsAllowed = []string{"internal/app", "cmd/lathe"}

	networkAllowed = []string{"internal/deps", "internal/update", "scripts/boundary"}

	networkPrefixes = []string{
		"net", "net/http", "net/url", "net/rpc", "net/smtp",
		"golang.org/x/net", "github.com/hashicorp/go-getter",
	}
)

func main() {
	var violations []string

	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", ".tools", "bin", "dist", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel := filepath.ToSlash(p)
		dir := path.Dir(rel)

		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", p, err)
		}
		for _, imp := range f.Imports {
			target, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if isWails(target) && !underAny(dir, wailsAllowed) {
				violations = append(violations,
					fmt.Sprintf("%s imports %s (Wails is confined to %s)", rel, target, strings.Join(wailsAllowed, ", ")))
			}
			if isNetwork(target) && !underAny(dir, networkAllowed) {
				violations = append(violations,
					fmt.Sprintf("%s imports %s (network access is confined to %s)", rel, target, strings.Join(networkAllowed, ", ")))
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "boundary:", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "import boundary violations:")
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, "  "+v)
		}
		os.Exit(1)
	}
	fmt.Println("import boundaries ok")
}

func isWails(target string) bool {
	return strings.HasPrefix(target, "github.com/wailsapp/wails/")
}

func isNetwork(target string) bool {
	// net/netip and net/textproto carry no dialing capability.
	switch target {
	case "net/netip", "net/textproto", "net/mail":
		return false
	}
	for _, p := range networkPrefixes {
		if target == p || strings.HasPrefix(target, p+"/") {
			return true
		}
	}
	return false
}

func underAny(dir string, roots []string) bool {
	for _, r := range roots {
		if dir == r || strings.HasPrefix(dir, r+"/") {
			return true
		}
	}
	return false
}
