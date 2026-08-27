package apiaudit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/strahe/synapse-go"

func TestPublicAPIHasNoInternalTypes(t *testing.T) {
	repoRoot := repositoryRoot(t)
	publicPackages := []string{
		".",
		"chain",
		"costs",
		"filbeam",
		"payments",
		"pdp",
		"piece",
		"sessionkey",
		"signer",
		"spregistry",
		"storage",
		"types",
		"warmstorage",
	}
	fset := token.NewFileSet()

	for _, packageDir := range publicPackages {
		dir := filepath.Join(repoRoot, packageDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read package directory %s: %v", packageDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			filename := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, filename, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", filename, err)
			}
			aliases := internalImportAliases(t, file, filename)
			checkExportedDeclarations(t, fset, file, aliases)
		}
	}
}

func TestExternalModuleCanConfigureServiceOptions(t *testing.T) {
	repoRoot := repositoryRoot(t)
	dir := t.TempDir()
	goMod := "module apiconfigtest\n\n" +
		"go 1.26.3\n\n" +
		"require " + modulePath + " v0.0.0\n\n" +
		"replace " + modulePath + " => " + filepath.ToSlash(repoRoot) + "\n"
	writeFile(t, filepath.Join(dir, "go.mod"), goMod)

	goSum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum"))
	if err != nil {
		t.Fatalf("read repository go.sum: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), goSum, 0o600); err != nil {
		t.Fatalf("write external go.sum: %v", err)
	}

	writeFile(t, filepath.Join(dir, "options_test.go"), `package apiconfigtest

import (
	"context"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

type nonceManager struct{}

func (nonceManager) Acquire(context.Context) (uint64, func(), error) {
	return 0, func() {}, nil
}

type lifecycle struct{}

func (lifecycle) CheckClosed() error { return nil }

var (
	sharedNonce     nonceManager
	sharedLifecycle lifecycle

	_ = payments.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = warmstorage.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = spregistry.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = sessionkey.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = costs.Options{Lifecycle: sharedLifecycle}
	_ = filbeam.Options{Lifecycle: sharedLifecycle}
	_ = storage.Options{Lifecycle: sharedLifecycle}
)
`)

	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external service Options did not compile: %v\n%s", err, out)
	}
}

func TestRootServiceWiringSharesRuntimeState(t *testing.T) {
	repoRoot := repositoryRoot(t)
	fset := token.NewFileSet()
	filename := filepath.Join(repoRoot, "services.go")
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse services.go: %v", err)
	}

	var initServices *ast.BlockStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "initServices" {
			initServices = fn.Body
			break
		}
	}
	if initServices == nil {
		t.Fatal("initServices not found")
	}

	type expectation struct {
		nonce     bool
		lifecycle bool
	}
	expected := map[string]expectation{
		"warmstorage": {nonce: true, lifecycle: true},
		"spregistry":  {nonce: true, lifecycle: true},
		"payments":    {nonce: true, lifecycle: true},
		"sessionkey":  {nonce: true, lifecycle: true},
		"filbeam":     {lifecycle: true},
		"costs":       {lifecycle: true},
		"storage":     {lifecycle: true},
	}
	seen := make(map[string]int, len(expected))

	ast.Inspect(initServices, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Options" {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		want, ok := expected[packageName.Name]
		if !ok {
			return true
		}
		seen[packageName.Name]++

		fields := make(map[string]ast.Expr, len(literal.Elts))
		for _, element := range literal.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			name, ok := field.Key.(*ast.Ident)
			if ok {
				fields[name.Name] = field.Value
			}
		}
		if want.nonce && !isClientSelector(fields["NonceManager"], "nonces") {
			t.Errorf("%s.Options.NonceManager must be c.nonces", packageName.Name)
		}
		if want.lifecycle && !isClientSelector(fields["Lifecycle"], "lifecycle") {
			t.Errorf("%s.Options.Lifecycle must be c.lifecycle", packageName.Name)
		}
		return true
	})

	for packageName := range expected {
		if seen[packageName] != 1 {
			t.Errorf("%s.Options initializers in initServices = %d, want 1", packageName, seen[packageName])
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func writeFile(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

func internalImportAliases(t *testing.T, file *ast.File, filename string) map[string]struct{} {
	t.Helper()
	aliases := make(map[string]struct{})
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", filename, err)
		}
		if !strings.HasPrefix(importPath, modulePath+"/internal/") {
			continue
		}
		alias := path.Base(importPath)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		switch alias {
		case "_":
			continue
		case ".":
			t.Fatalf("cannot audit dot import of internal package in %s", filename)
		default:
			aliases[alias] = struct{}{}
		}
	}
	return aliases
}

func checkExportedDeclarations(t *testing.T, fset *token.FileSet, file *ast.File, aliases map[string]struct{}) {
	t.Helper()
	check := func(label string, expr ast.Expr) {
		if referencesInternalImport(expr, aliases) {
			t.Errorf("%s: %s references an internal package type", fset.Position(expr.Pos()), label)
		}
	}

	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Name.IsExported() {
				check("exported function or method "+decl.Name.Name, decl.Type)
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if !spec.Name.IsExported() {
						continue
					}
					if spec.TypeParams != nil {
						for _, field := range spec.TypeParams.List {
							check("type parameters of "+spec.Name.Name, field.Type)
						}
					}
					structure, ok := spec.Type.(*ast.StructType)
					if !ok {
						check("exported type "+spec.Name.Name, spec.Type)
						continue
					}
					for _, field := range structure.Fields.List {
						if len(field.Names) == 0 {
							check("embedded field of "+spec.Name.Name, field.Type)
							continue
						}
						for _, name := range field.Names {
							if name.IsExported() {
								check("exported field "+spec.Name.Name+"."+name.Name, field.Type)
								break
							}
						}
					}
				case *ast.ValueSpec:
					if spec.Type == nil {
						continue
					}
					for _, name := range spec.Names {
						if name.IsExported() {
							check("explicit type of exported value "+name.Name, spec.Type)
							break
						}
					}
				}
			}
		}
	}
}

func referencesInternalImport(expr ast.Expr, aliases map[string]struct{}) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, ok := aliases[identifier.Name]; ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func isClientSelector(expr ast.Expr, field string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != field {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == "c"
}
