package test

import (
	"strings"
	"testing"
)

func TestInternalTestPackageIsNotImported(t *testing.T) {
	packages := listPackages(t)
	for _, pkg := range packages {
		if strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/test") {
			continue
		}

		for _, imported := range allImports(pkg) {
			if imported == modulePath+"/internal/test" ||
				strings.HasPrefix(imported, modulePath+"/internal/test/") {
				t.Errorf("internal test package must not be imported: %s imports %s", pkg.ImportPath, imported)
			}
		}
	}
}
