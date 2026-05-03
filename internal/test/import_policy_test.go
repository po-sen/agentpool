package test

import (
	"strings"
	"testing"
)

func TestProductionImportsStayWithinArchitectureBoundaries(t *testing.T) {
	packages := listPackages(t)

	rules := []importRule{
		{
			name:          "cmd stays thin",
			packagePrefix: modulePath + "/cmd/",
			forbiddenImports: []string{
				modulePath + "/internal/application",
				modulePath + "/internal/config",
				modulePath + "/internal/domain",
				modulePath + "/internal/infrastructure",
				modulePath + "/internal/runtime",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "domain stays pure",
			packagePrefix: modulePath + "/internal/domain/",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "application does not depend on delivery or infrastructure",
			packagePrefix: modulePath + "/internal/application/",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "inbound application contracts stay domain-free",
			packagePrefix: modulePath + "/internal/application/port/inbound",
			forbiddenImports: []string{
				modulePath + "/internal/domain",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
			},
		},
		{
			name:          "outbound application ports stay implementation-free",
			packagePrefix: modulePath + "/internal/application/port/outbound",
			forbiddenImports: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
			},
		},
		{
			name:          "application commands do not depend on other use case implementations",
			packagePrefix: modulePath + "/internal/application/command",
			forbiddenImports: []string{
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
			},
		},
		{
			name:          "application queries do not depend on other use case implementations",
			packagePrefix: modulePath + "/internal/application/query",
			forbiddenImports: []string{
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/workflow",
			},
		},
		{
			name:          "application workflows do not depend on command or query handlers",
			packagePrefix: modulePath + "/internal/application/workflow",
			forbiddenImports: []string{
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/query",
			},
		},
		{
			name:          "application agent does not depend on other application flows",
			packagePrefix: modulePath + "/internal/application/agent",
			forbiddenImports: []string{
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
			},
		},
		{
			name:          "delivery uses inbound application contracts only",
			packagePrefix: modulePath + "/internal/delivery/",
			forbiddenImports: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/domain",
				modulePath + "/internal/infrastructure",
				"database/sql",
			},
		},
		{
			name:          "infrastructure does not depend on delivery or use cases",
			packagePrefix: modulePath + "/internal/infrastructure/",
			forbiddenImports: []string{
				modulePath + "/internal/delivery",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
			},
		},
		{
			name:          "runtime helpers stay product agnostic",
			packagePrefix: modulePath + "/internal/runtime/",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/domain/",
				modulePath + "/internal/infrastructure/",
			},
		},
	}

	assertForbiddenImports(t, packages, rules, func(pkg listedPackage) []string {
		return pkg.Imports
	})
}

func TestUnitTestImportsStayWithinPackageBoundaries(t *testing.T) {
	packages := listPackages(t)

	rules := []importRule{
		{
			name:          "domain unit tests stay domain-only",
			packagePrefix: modulePath + "/internal/domain/",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "inbound port unit tests stay contract-only",
			packagePrefix: modulePath + "/internal/application/port/inbound",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/domain",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "outbound port unit tests stay implementation-free",
			packagePrefix: modulePath + "/internal/application/port/outbound",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "application command unit tests do not use infrastructure or other use cases",
			packagePrefix: modulePath + "/internal/application/command",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "application query unit tests do not use infrastructure or other use cases",
			packagePrefix: modulePath + "/internal/application/query",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "application workflow unit tests do not use infrastructure or command/query handlers",
			packagePrefix: modulePath + "/internal/application/workflow",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/query",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "application agent unit tests do not use infrastructure or other flows",
			packagePrefix: modulePath + "/internal/application/agent",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
				"database/sql",
				"net/http",
			},
		},
		{
			name:          "delivery unit tests use inbound application contracts only",
			packagePrefix: modulePath + "/internal/delivery/",
			forbiddenImports: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/domain",
				modulePath + "/internal/infrastructure",
				"database/sql",
			},
		},
		{
			name:          "infrastructure unit tests do not depend on delivery or use cases",
			packagePrefix: modulePath + "/internal/infrastructure/",
			forbiddenImports: []string{
				modulePath + "/internal/delivery",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/query",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/bootstrap",
			},
		},
		{
			name:          "runtime unit tests stay product agnostic",
			packagePrefix: modulePath + "/internal/runtime/",
			forbiddenImports: []string{
				modulePath + "/cmd/",
				modulePath + "/internal/application/",
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/domain/",
				modulePath + "/internal/infrastructure/",
			},
		},
	}

	assertForbiddenImports(t, packages, rules, testImports)
}

func TestDomainImportsStayCentralized(t *testing.T) {
	packages := listPackages(t)
	allowedImporters := []string{
		modulePath + "/internal/domain/",
		modulePath + "/internal/application/agent",
		modulePath + "/internal/application/command",
		modulePath + "/internal/application/query",
		modulePath + "/internal/application/workflow",
		modulePath + "/internal/application/port/outbound",
		modulePath + "/internal/infrastructure/",
	}

	for _, pkg := range packages {
		for _, imported := range pkg.Imports {
			if !isForbiddenImport(imported, []string{modulePath + "/internal/domain"}) {
				continue
			}
			if hasAllowedPrefix(pkg.ImportPath, allowedImporters) {
				continue
			}

			t.Errorf("domain import is not allowlisted: %s imports %s", pkg.ImportPath, imported)
		}
	}
}

func TestDomainConceptsDoNotImportEachOther(t *testing.T) {
	packages := listPackages(t)

	for _, pkg := range packages {
		if !strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/domain/") {
			continue
		}

		concept, ok := domainConcept(pkg.ImportPath)
		if !ok {
			continue
		}

		for _, imported := range pkg.Imports {
			importedConcept, ok := domainConcept(imported)
			if !ok || importedConcept == concept {
				continue
			}

			t.Errorf("domain concept %q must not import domain concept %q: %s imports %s",
				concept,
				importedConcept,
				pkg.ImportPath,
				imported,
			)
		}
	}
}

func assertForbiddenImports(
	t *testing.T,
	packages []listedPackage,
	rules []importRule,
	importsFor func(listedPackage) []string,
) {
	t.Helper()

	for _, pkg := range packages {
		for _, rule := range rules {
			if !matchesPackagePrefix(pkg.ImportPath, rule.packagePrefix) {
				continue
			}

			for _, imported := range importsFor(pkg) {
				if isForbiddenImport(imported, rule.forbiddenImports) {
					t.Errorf("%s: %s imports forbidden package %s", rule.name, pkg.ImportPath, imported)
				}
			}
		}
	}
}

func domainConcept(importPath string) (string, bool) {
	prefix := modulePath + "/internal/domain/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}

	remainder := strings.TrimPrefix(importPath, prefix)
	concept, _, _ := strings.Cut(remainder, "/")
	if concept == "" {
		return "", false
	}

	return concept, true
}
