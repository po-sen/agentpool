package test

import (
	"strings"
	"testing"
)

func TestInternalPackageTopologyIsExplicitlyAllowlisted(t *testing.T) {
	allowedPackages := map[string]struct{}{
		modulePath + "/internal/adapters/inbound/cli":      {},
		modulePath + "/internal/adapters/inbound/httpapi":  {},
		modulePath + "/internal/adapters/outbound/agent":   {},
		modulePath + "/internal/adapters/outbound/events":  {},
		modulePath + "/internal/adapters/outbound/git":     {},
		modulePath + "/internal/adapters/outbound/ids":     {},
		modulePath + "/internal/adapters/outbound/memory":  {},
		modulePath + "/internal/adapters/outbound/policy":  {},
		modulePath + "/internal/adapters/outbound/sandbox": {},
		modulePath + "/internal/adapters/outbound/secrets": {},
		modulePath + "/internal/application/command":       {},
		modulePath + "/internal/application/port/inbound":  {},
		modulePath + "/internal/application/port/outbound": {},
		modulePath + "/internal/application/query":         {},
		modulePath + "/internal/application/workflow":      {},
		modulePath + "/internal/bootstrap":                 {},
		modulePath + "/internal/config":                    {},
		modulePath + "/internal/domain/approval":           {},
		modulePath + "/internal/domain/run":                {},
		modulePath + "/internal/runtime/httpserver":        {},
		modulePath + "/internal/runtime/logger":            {},
		modulePath + "/internal/test":                      {},
	}

	for _, pkg := range listPackages(t) {
		if !strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/") {
			continue
		}
		if _, ok := allowedPackages[pkg.ImportPath]; ok {
			continue
		}

		t.Errorf("internal package is not allowlisted: %s", pkg.ImportPath)
	}
}

func TestDisallowedCatchAllPackagesDoNotExist(t *testing.T) {
	disallowedNames := map[string]struct{}{
		"common":      {},
		"dto":         {},
		"entity":      {},
		"helper":      {},
		"helpers":     {},
		"model":       {},
		"models":      {},
		"shared":      {},
		"util":        {},
		"utils":       {},
		"valueobject": {},
	}

	for _, pkg := range listPackages(t) {
		if !strings.HasPrefix(pkg.ImportPath, modulePath+"/internal/") {
			continue
		}

		parts := strings.Split(strings.TrimPrefix(pkg.ImportPath, modulePath+"/internal/"), "/")
		for _, part := range parts {
			if _, ok := disallowedNames[part]; !ok {
				continue
			}

			t.Errorf("catch-all package name is not allowed: %s contains %q", pkg.ImportPath, part)
		}
	}
}
