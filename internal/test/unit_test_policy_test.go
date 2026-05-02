package test

import (
	"strings"
	"testing"
)

func TestRequiredPackagesHaveUnitTests(t *testing.T) {
	packages := listPackages(t)
	requiredPrefixes := []string{
		modulePath + "/internal/domain/",
		modulePath + "/internal/application/port/inbound",
		modulePath + "/internal/application/port/outbound",
		modulePath + "/internal/application/command",
		modulePath + "/internal/application/query",
		modulePath + "/internal/application/workflow",
		modulePath + "/internal/bootstrap",
		modulePath + "/internal/config",
		modulePath + "/internal/delivery/",
		modulePath + "/internal/infrastructure/",
		modulePath + "/internal/runtime/",
	}

	for _, pkg := range packages {
		if len(pkg.GoFiles) == 0 || !hasAllowedPrefix(pkg.ImportPath, requiredPrefixes) {
			continue
		}
		if len(pkg.TestGoFiles)+len(pkg.XTestGoFiles) > 0 {
			continue
		}

		t.Errorf("required unit test package has no tests: %s", pkg.ImportPath)
	}
}

func TestInternalProductionFilesHaveCompanionUnitTests(t *testing.T) {
	packages := listPackages(t)

	for _, pkg := range packages {
		if !requiresCompanionUnitTests(pkg.ImportPath) {
			continue
		}

		testFiles := append([]string{}, pkg.TestGoFiles...)
		testFiles = append(testFiles, pkg.XTestGoFiles...)
		for _, file := range pkg.GoFiles {
			companion := strings.TrimSuffix(file, ".go") + "_test.go"
			if containsString(testFiles, companion) {
				continue
			}

			t.Errorf("production file has no companion unit test: %s/%s wants %s",
				pkg.ImportPath,
				file,
				companion,
			)
		}
	}
}

func requiresCompanionUnitTests(importPath string) bool {
	if importPath == modulePath+"/internal/test" ||
		strings.HasPrefix(importPath, modulePath+"/internal/test/") {
		return false
	}

	return strings.HasPrefix(importPath, modulePath+"/internal/")
}
