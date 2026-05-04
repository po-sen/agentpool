package test

import (
	"strings"
	"testing"
)

var (
	databasePackages = []string{
		"database/sql",
	}
	technologyBoundaryPackages = []string{
		"database/sql",
		"net/http",
	}
)

func TestProductionInternalImportsStayWithinArchitectureBoundaries(t *testing.T) {
	packages := listPackages(t)

	rules := []internalImportRule{
		{
			name:           "cmd stays thin",
			importerPrefix: modulePath + "/cmd/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/bootstrap",
				modulePath + "/internal/delivery/cli",
			},
		},
		{
			name:           "bootstrap owns concrete wiring",
			importerPrefix: modulePath + "/internal/bootstrap",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/",
				modulePath + "/internal/config",
				modulePath + "/internal/delivery/",
				modulePath + "/internal/infrastructure/",
				modulePath + "/internal/runtime/",
			},
		},
		{
			name:           "config stays independent",
			importerPrefix: modulePath + "/internal/config",
		},
		{
			name:           "domain stays within domain",
			importerPrefix: modulePath + "/internal/domain/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application stays inside application and domain",
			importerPrefix: modulePath + "/internal/application/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "inbound application contracts stay domain-free",
			importerPrefix: modulePath + "/internal/application/port/inbound",
		},
		{
			name:           "outbound application ports expose only domain-facing contracts",
			importerPrefix: modulePath + "/internal/application/port/outbound",
			allowedImportPrefixes: []string{
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application commands depend only on contracts and domain",
			importerPrefix: modulePath + "/internal/application/command",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application queries depend only on contracts and domain",
			importerPrefix: modulePath + "/internal/application/query",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application workflows depend only on agent, contracts, and domain",
			importerPrefix: modulePath + "/internal/application/workflow",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application agent depends only on outbound contracts and domain",
			importerPrefix: modulePath + "/internal/application/agent",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "delivery uses inbound application contracts only",
			importerPrefix: modulePath + "/internal/delivery/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/delivery/",
			},
		},
		{
			name:           "infrastructure depends only on outbound contracts and domain",
			importerPrefix: modulePath + "/internal/infrastructure/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
				modulePath + "/internal/infrastructure/",
			},
		},
		{
			name:           "runtime helpers stay product agnostic",
			importerPrefix: modulePath + "/internal/runtime/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/runtime/",
			},
		},
	}

	assertInternalImports(t, packages, rules, func(pkg listedPackage) []string {
		return pkg.Imports
	})
}

func TestProductionExternalImportsStayWithinLayerBoundaries(t *testing.T) {
	packages := listPackages(t)
	standardLibraryPackages := listStandardLibraryPackages(t)

	rules := []externalImportRule{
		{
			name:                             "cmd uses only standard library at the process boundary",
			importerPrefix:                   modulePath + "/cmd/",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "bootstrap uses only standard library while wiring",
			importerPrefix:                   modulePath + "/internal/bootstrap",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "config uses only standard library",
			importerPrefix:                   modulePath + "/internal/config",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "domain uses only standard library",
			importerPrefix:                   modulePath + "/internal/domain/",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "application uses only standard library",
			importerPrefix:                   modulePath + "/internal/application/",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "delivery must not use database packages",
			importerPrefix:                   modulePath + "/internal/delivery/",
			allowThirdParty:                  true,
			forbiddenStandardLibraryPackages: databasePackages,
		},
		{
			name:            "infrastructure may use third-party integrations",
			importerPrefix:  modulePath + "/internal/infrastructure/",
			allowThirdParty: true,
		},
		{
			name:           "runtime uses only standard library",
			importerPrefix: modulePath + "/internal/runtime/",
		},
	}

	assertExternalImports(t, packages, rules, standardLibraryPackages, func(pkg listedPackage) []string {
		return pkg.Imports
	})
}

func TestUnitTestInternalImportsStayWithinPackageBoundaries(t *testing.T) {
	packages := listPackages(t)

	rules := []internalImportRule{
		{
			name:           "domain unit tests stay domain-only",
			importerPrefix: modulePath + "/internal/domain/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application unit tests stay inside application and domain",
			importerPrefix: modulePath + "/internal/application/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "inbound port unit tests stay contract-only",
			importerPrefix: modulePath + "/internal/application/port/inbound",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
			},
		},
		{
			name:           "outbound port unit tests stay implementation-free",
			importerPrefix: modulePath + "/internal/application/port/outbound",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application command unit tests use only command contracts and domain",
			importerPrefix: modulePath + "/internal/application/command",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/command",
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application query unit tests use only query contracts and domain",
			importerPrefix: modulePath + "/internal/application/query",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/query",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application workflow unit tests use only workflow contracts and domain",
			importerPrefix: modulePath + "/internal/application/workflow",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/application/workflow",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "application agent unit tests use only agent contracts and domain",
			importerPrefix: modulePath + "/internal/application/agent",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/agent",
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
			},
		},
		{
			name:           "bootstrap unit tests stay at the wiring boundary",
			importerPrefix: modulePath + "/internal/bootstrap",
			allowedImportPrefixes: []string{
				modulePath + "/internal/bootstrap",
			},
		},
		{
			name:           "config unit tests stay independent",
			importerPrefix: modulePath + "/internal/config",
			allowedImportPrefixes: []string{
				modulePath + "/internal/config",
			},
		},
		{
			name:           "delivery unit tests use inbound application contracts only",
			importerPrefix: modulePath + "/internal/delivery/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/inbound",
				modulePath + "/internal/delivery/",
			},
		},
		{
			name:           "infrastructure unit tests stay on infrastructure contracts",
			importerPrefix: modulePath + "/internal/infrastructure/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/application/port/outbound",
				modulePath + "/internal/domain/",
				modulePath + "/internal/infrastructure/",
			},
		},
		{
			name:           "runtime unit tests stay product agnostic",
			importerPrefix: modulePath + "/internal/runtime/",
			allowedImportPrefixes: []string{
				modulePath + "/internal/runtime/",
			},
		},
	}

	assertInternalImports(t, packages, rules, testImports)
}

func TestUnitTestExternalImportsStayWithinLayerBoundaries(t *testing.T) {
	packages := listPackages(t)
	standardLibraryPackages := listStandardLibraryPackages(t)

	rules := []externalImportRule{
		{
			name:                             "domain unit tests use only standard library",
			importerPrefix:                   modulePath + "/internal/domain/",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "application unit tests use only standard library",
			importerPrefix:                   modulePath + "/internal/application/",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "bootstrap unit tests use only standard library",
			importerPrefix:                   modulePath + "/internal/bootstrap",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "config unit tests use only standard library",
			importerPrefix:                   modulePath + "/internal/config",
			forbiddenStandardLibraryPackages: technologyBoundaryPackages,
		},
		{
			name:                             "delivery unit tests must not use database packages",
			importerPrefix:                   modulePath + "/internal/delivery/",
			allowThirdParty:                  true,
			forbiddenStandardLibraryPackages: databasePackages,
		},
		{
			name:            "infrastructure unit tests may use third-party integrations",
			importerPrefix:  modulePath + "/internal/infrastructure/",
			allowThirdParty: true,
		},
		{
			name:           "runtime unit tests use only standard library",
			importerPrefix: modulePath + "/internal/runtime/",
		},
	}

	assertExternalImports(t, packages, rules, standardLibraryPackages, testImports)
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

func assertInternalImports(
	t *testing.T,
	packages []listedPackage,
	rules []internalImportRule,
	importsFor func(listedPackage) []string,
) {
	t.Helper()

	for _, pkg := range packages {
		for _, rule := range rules {
			if !matchesPackagePrefix(pkg.ImportPath, rule.importerPrefix) {
				continue
			}

			for _, imported := range importsFor(pkg) {
				assertInternalImportAllowed(t, rule, pkg.ImportPath, imported)
			}
		}
	}
}

func assertInternalImportAllowed(t *testing.T, rule internalImportRule, importer string, imported string) {
	t.Helper()

	if !isModuleImport(imported) {
		return
	}
	if hasAllowedPrefix(imported, rule.allowedImportPrefixes) {
		return
	}

	t.Errorf("%s: %s imports unallowed internal package %s", rule.name, importer, imported)
}

func assertExternalImports(
	t *testing.T,
	packages []listedPackage,
	rules []externalImportRule,
	standardLibraryPackages map[string]struct{},
	importsFor func(listedPackage) []string,
) {
	t.Helper()

	for _, pkg := range packages {
		for _, rule := range rules {
			if !matchesPackagePrefix(pkg.ImportPath, rule.importerPrefix) {
				continue
			}

			for _, imported := range importsFor(pkg) {
				if isModuleImport(imported) {
					continue
				}

				assertExternalImportAllowed(t, rule, pkg.ImportPath, imported, standardLibraryPackages)
			}
		}
	}
}

func assertExternalImportAllowed(
	t *testing.T,
	rule externalImportRule,
	importer string,
	imported string,
	standardLibraryPackages map[string]struct{},
) {
	t.Helper()

	if isStandardLibraryImport(imported, standardLibraryPackages) {
		if hasAllowedPrefix(imported, rule.forbiddenStandardLibraryPackages) {
			t.Errorf("%s: %s imports forbidden standard library package %s", rule.name, importer, imported)
		}

		return
	}

	if rule.allowThirdParty {
		return
	}

	t.Errorf("%s: %s imports third-party package %s", rule.name, importer, imported)
}

func isModuleImport(importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func isStandardLibraryImport(importPath string, standardLibraryPackages map[string]struct{}) bool {
	_, ok := standardLibraryPackages[importPath]

	return ok
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
