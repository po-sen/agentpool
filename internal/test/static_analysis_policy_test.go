package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	maxFunctionParameters                = 7
	maxAllowedStringLiteralOccurrences   = 2
	minDuplicatedStringLiteralValueBytes = 5
)

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

func TestProductionGoFilesDoNotDuplicateStringLiterals(t *testing.T) {
	for _, pkg := range listPackages(t) {
		for _, file := range pkg.GoFiles {
			path := filepath.Join(pkg.Dir, file)
			parsed := parseGoFile(t, path)

			occurrencesByLiteral := stringLiteralOccurrences(parsed)
			for _, literal := range sortedStringLiterals(occurrencesByLiteral) {
				occurrences := occurrencesByLiteral[literal]
				if len(occurrences) <= maxAllowedStringLiteralOccurrences {
					continue
				}

				t.Errorf("%s duplicates string literal %q %d times; define a constant and reuse it: %s",
					path,
					literal,
					len(occurrences),
					strings.Join(occurrences, ", "),
				)
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

func stringLiteralOccurrences(parsed parsedGoFile) map[string][]string {
	ignoredPositions := ignoredStringLiteralPositions(parsed.File)
	occurrences := make(map[string][]string)

	ast.Inspect(parsed.File, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if _, ignored := ignoredPositions[lit.Pos()]; ignored {
			return true
		}

		value, err := strconv.Unquote(lit.Value)
		if err != nil || len(value) < minDuplicatedStringLiteralValueBytes {
			return true
		}

		position := parsed.FileSet.Position(lit.Pos()).String()
		occurrences[value] = append(occurrences[value], position)

		return true
	})

	return occurrences
}

func sortedStringLiterals(occurrences map[string][]string) []string {
	literals := make([]string, 0, len(occurrences))
	for literal := range occurrences {
		literals = append(literals, literal)
	}
	sort.Strings(literals)

	return literals
}

func ignoredStringLiteralPositions(file *ast.File) map[token.Pos]struct{} {
	positions := make(map[token.Pos]struct{})
	for _, imported := range file.Imports {
		positions[imported.Path.Pos()] = struct{}{}
	}

	// Struct tags are syntax metadata and cannot use constants.
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || field.Tag == nil {
			return true
		}

		positions[field.Tag.Pos()] = struct{}{}

		return true
	})

	return positions
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
