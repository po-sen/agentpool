package test

import (
	"strings"
	"testing"
)

func TestInternalPackageTopologyIsExplicitlyAllowlisted(t *testing.T) {
	allowedPackages := map[string]struct{}{
		modulePath + "/internal/application/agent":                    {},
		modulePath + "/internal/application/command":                  {},
		modulePath + "/internal/application/port/inbound":             {},
		modulePath + "/internal/application/port/outbound":            {},
		modulePath + "/internal/application/query":                    {},
		modulePath + "/internal/application/workflow":                 {},
		modulePath + "/internal/bootstrap":                            {},
		modulePath + "/internal/config":                               {},
		modulePath + "/internal/delivery/cli":                         {},
		modulePath + "/internal/delivery/httpapi":                     {},
		modulePath + "/internal/domain/approval":                      {},
		modulePath + "/internal/domain/run":                           {},
		modulePath + "/internal/infrastructure/event/noop":            {},
		modulePath + "/internal/infrastructure/git/noop":              {},
		modulePath + "/internal/infrastructure/id/crypto":             {},
		modulePath + "/internal/infrastructure/llm/anthropic":         {},
		modulePath + "/internal/infrastructure/llm/gemini":            {},
		modulePath + "/internal/infrastructure/llm/noop":              {},
		modulePath + "/internal/infrastructure/llm/openai":            {},
		modulePath + "/internal/infrastructure/llm/openai_compatible": {},
		modulePath + "/internal/infrastructure/persistence/memory":    {},
		modulePath + "/internal/infrastructure/policy/allowall":       {},
		modulePath + "/internal/infrastructure/sandbox/noop":          {},
		modulePath + "/internal/infrastructure/secret/noop":           {},
		modulePath + "/internal/infrastructure/tool/composite":        {},
		modulePath + "/internal/infrastructure/tool/file":             {},
		modulePath + "/internal/infrastructure/tool/shell":            {},
		modulePath + "/internal/infrastructure/workspace/temp":        {},
		modulePath + "/internal/runtime/httpserver":                   {},
		modulePath + "/internal/runtime/logger":                       {},
		modulePath + "/internal/test":                                 {},
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
