package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/po-sen/agentpool"

type listedPackage struct {
	ImportPath   string
	GoFiles      []string
	Imports      []string
	TestGoFiles  []string
	TestImports  []string
	XTestGoFiles []string
	XTestImports []string
}

type importRule struct {
	name             string
	packagePrefix    string
	forbiddenImports []string
}

func listPackages(t *testing.T) []listedPackage {
	t.Helper()

	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = moduleRoot(t)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list packages: %v\n%s", err, stderr.String())
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []listedPackage
	for {
		var pkg listedPackage
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}

		if pkg.ImportPath == "" {
			t.Fatal("go list returned a package without an import path")
		}

		packages = append(packages, pkg)
	}

	if len(packages) == 0 {
		t.Fatal("go list returned no packages")
	}

	return packages
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	command := exec.Command("go", "env", "GOMOD")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}

	gomod := strings.TrimSpace(string(output))
	if gomod == "" {
		t.Fatal("go env GOMOD returned an empty path")
	}

	return filepath.Dir(gomod)
}

func isForbiddenImport(imported string, forbidden []string) bool {
	for _, prefix := range forbidden {
		if imported == prefix || strings.HasPrefix(imported, fmt.Sprintf("%s/", prefix)) {
			return true
		}
	}

	return false
}

func matchesPackagePrefix(importPath string, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(importPath, prefix)
	}

	return importPath == prefix || strings.HasPrefix(importPath, fmt.Sprintf("%s/", prefix))
}

func hasAllowedPrefix(importPath string, allowed []string) bool {
	for _, prefix := range allowed {
		if strings.HasSuffix(prefix, "/") {
			if strings.HasPrefix(importPath, prefix) {
				return true
			}

			continue
		}
		if importPath == prefix || strings.HasPrefix(importPath, fmt.Sprintf("%s/", prefix)) {
			return true
		}
	}

	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}

	return false
}

func allImports(pkg listedPackage) []string {
	imports := make([]string, 0, len(pkg.Imports)+len(pkg.TestImports)+len(pkg.XTestImports))
	imports = append(imports, pkg.Imports...)
	imports = append(imports, pkg.TestImports...)
	imports = append(imports, pkg.XTestImports...)

	return imports
}

func testImports(pkg listedPackage) []string {
	imports := make([]string, 0, len(pkg.TestImports)+len(pkg.XTestImports))
	imports = append(imports, pkg.TestImports...)
	imports = append(imports, pkg.XTestImports...)

	return imports
}
