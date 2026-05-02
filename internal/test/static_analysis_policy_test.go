package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

const maxFunctionParameters = 7

func TestGoFunctionsDoNotExceedParameterLimit(t *testing.T) {
	for _, pkg := range listPackages(t) {
		for _, file := range packageGoFiles(pkg) {
			path := filepath.Join(pkg.Dir, file)
			parsed := parseGoFile(t, path)

			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				count := parameterCount(fn.Type.Params)
				if count <= maxFunctionParameters {
					continue
				}

				position := parsed.FileSet.Position(fn.Pos())
				t.Errorf("%s has %d parameters, max allowed is %d", position, count, maxFunctionParameters)
			}
		}
	}
}

type parsedGoFile struct {
	*ast.File
	FileSet *token.FileSet
}

func parseGoFile(t *testing.T, path string) parsedGoFile {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("parse Go file %s: %v", path, err)
	}

	return parsedGoFile{
		File:    file,
		FileSet: fileSet,
	}
}

func packageGoFiles(pkg listedPackage) []string {
	files := make([]string, 0, len(pkg.GoFiles)+len(pkg.TestGoFiles)+len(pkg.XTestGoFiles))
	files = append(files, pkg.GoFiles...)
	files = append(files, pkg.TestGoFiles...)
	files = append(files, pkg.XTestGoFiles...)

	return files
}

func parameterCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}

	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}

		count += len(field.Names)
	}

	return count
}
