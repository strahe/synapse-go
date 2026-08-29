package apiaudit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/strahe/synapse-go"

func TestPublicAPIHasNoInternalTypes(t *testing.T) {
	repoRoot := repositoryRoot(t)
	packages := listPackages(t, repoRoot)
	exports := make(map[string]string, len(packages))
	publicPackages := make([]listedPackage, 0, len(packages))
	for _, pkg := range packages {
		if pkg.Export != "" {
			exports[pkg.ImportPath] = pkg.Export
		}
		if isPublicPackage(pkg) {
			publicPackages = append(publicPackages, pkg)
		}
	}
	sort.Slice(publicPackages, func(i, j int) bool {
		return publicPackages[i].ImportPath < publicPackages[j].ImportPath
	})

	fset := token.NewFileSet()
	lookup := func(importPath string) (io.ReadCloser, error) {
		exportFile, ok := exports[importPath]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", importPath)
		}
		return os.Open(exportFile)
	}
	compiledImporter := importer.ForCompiler(fset, "gc", lookup)

	for _, pkg := range publicPackages {
		files := parsePackageFiles(t, fset, pkg)
		info := &types.Info{
			Defs:  make(map[*ast.Ident]types.Object),
			Types: make(map[ast.Expr]types.TypeAndValue),
		}
		config := types.Config{Importer: compiledImporter}
		if _, err := config.Check(pkg.ImportPath, fset, files, info); err != nil {
			t.Fatalf("type-check %s: %v", pkg.ImportPath, err)
		}
		checkExportedDeclarations(t, fset, files, info)
	}
}

func TestInternalTypeDetectionFollowsAliases(t *testing.T) {
	internalPackage := types.NewPackage(modulePath+"/internal/fake", "fake")
	hiddenName := types.NewTypeName(token.NoPos, internalPackage, "Hidden", nil)
	hidden := types.NewNamed(hiddenName, types.NewStruct(nil, nil), nil)

	publicPackage := types.NewPackage(modulePath+"/public", "public")
	aliasName := types.NewTypeName(token.NoPos, publicPackage, "leaked", nil)
	alias := types.NewAlias(aliasName, hidden)

	if got := findInternalType(alias); got != internalPackage.Path() {
		t.Fatalf("findInternalType(alias) = %q, want %q", got, internalPackage.Path())
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
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type nonceManager struct{}

func (nonceManager) Acquire(context.Context) (uint64, func(), error) {
	return 0, func() {}, nil
}

type lifecycle struct{}

func (lifecycle) CheckClosed() error { return nil }

type blockNumberReader struct{}

func (blockNumberReader) BlockNumber(context.Context) (uint64, error) { return 0, nil }

type endorsedProviderSource struct{}

func (endorsedProviderSource) GetEndorsedProviderIDs(context.Context) ([]types.BigInt, error) {
	return []types.BigInt{types.NewBigInt(1)}, nil
}

var (
	sharedNonce     nonceManager
	sharedLifecycle lifecycle
	sharedBuffer    int64

	_ = payments.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = warmstorage.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = spregistry.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ = sessionkey.Options{NonceManager: sharedNonce, Lifecycle: sharedLifecycle}
	_ costs.ContractCaller = blockNumberReader{}
	_                      = costs.Options{Caller: blockNumberReader{}, Lifecycle: sharedLifecycle}
	_                      = costs.UploadCostOptions{BufferEpochs: &sharedBuffer}
	_ = filbeam.Options{Lifecycle: sharedLifecycle}
	_ = storage.Options{Lifecycle: sharedLifecycle}
	_ storage.EndorsedProviderSource = endorsedProviderSource{}
	_ = storage.ServiceResolverOptions{Endorsements: endorsedProviderSource{}}
	_ = storage.UploadOptions{AllowUnendorsedPrimary: true}
	_ = storage.SelectUploadContextsOptions{AllowUnendorsedPrimary: true}
	_ = storage.MultiCostOptions{BufferEpochs: &sharedBuffer}
	_ = storage.PrepareOptions{BufferEpochs: &sharedBuffer}
	_ *storage.DataSetDetails
)
`)

	writeFile(t, filepath.Join(dir, "storage_signer_test.go"), `package apiconfigtest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

type kmsStorageSigner struct {
	key     *ecdsa.PrivateKey
	digests [][]byte
}

var _ signer.HashSigner = (*kmsStorageSigner)(nil)
var _ signer.StorageSigner = (*kmsStorageSigner)(nil)

func (s *kmsStorageSigner) EVMAddress() common.Address {
	return ethcrypto.PubkeyToAddress(s.key.PublicKey)
}

func (s *kmsStorageSigner) SignHash(hash []byte) ([]byte, error) {
	s.digests = append(s.digests, bytes.Clone(hash))
	return ethcrypto.Sign(hash, s.key)
}

type unusedPDPClient struct {
	storage.PDPProviderClient
}

func TestStorageSignerContract(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	kms := &kmsStorageSigner{key: key}
	if _, ok := any(kms).(signer.Signer); ok {
		t.Fatal("KMS signer unexpectedly implements signer.Signer")
	}
	if _, ok := any(kms).(signer.EVMSigner); ok {
		t.Fatal("KMS signer unexpectedly implements signer.EVMSigner")
	}

	_ = synapse.WithStorageSigner(kms)
	_ = storage.Options{Signer: kms}

	ctx, err := storage.NewProviderContext(
		storage.Provider{
			ID:              types.NewBigInt(1),
			ServiceURL:      "https://pdp.example.com",
			ServiceProvider: common.HexToAddress("0x1001"),
			Payee:           common.HexToAddress("0x1002"),
		},
		&unusedPDPClient{},
		kms,
		storage.WithPayer(common.HexToAddress("0x2001")),
		storage.WithChainID(types.ChainID(314159)),
		storage.WithRecordKeeper(common.HexToAddress("0x2002")),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	pieceInfo, err := piece.CalculateFromBytes(bytes.Repeat([]byte{0x42}, 256))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	extraData, err := ctx.PresignForCommit(context.Background(), []storage.PieceInput{{PieceCID: pieceInfo.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	if len(extraData) == 0 {
		t.Fatal("PresignForCommit returned empty extra data")
	}
	if len(kms.digests) != 2 {
		t.Fatalf("SignHash calls=%d want 2", len(kms.digests))
	}
	for i, digest := range kms.digests {
		if len(digest) != 32 {
			t.Fatalf("digest %d length=%d want 32", i, len(digest))
		}
	}
}
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

type listedPackage struct {
	ImportPath string
	Name       string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Export     string
}

func listPackages(t *testing.T, repoRoot string) []listedPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "-deps", "-export", "./...")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list packages: %v\n%s", err, out)
	}

	decoder := json.NewDecoder(bytes.NewReader(out))
	var packages []listedPackage
	for {
		var pkg listedPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode package list: %v", err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func isPublicPackage(pkg listedPackage) bool {
	if pkg.Name == "main" {
		return false
	}
	if pkg.ImportPath != modulePath && !strings.HasPrefix(pkg.ImportPath, modulePath+"/") {
		return false
	}
	if isModuleInternalPath(pkg.ImportPath) {
		return false
	}
	relative := strings.TrimPrefix(pkg.ImportPath, modulePath)
	return relative != "/examples" &&
		!strings.HasPrefix(relative, "/examples/") &&
		relative != "/tests" &&
		!strings.HasPrefix(relative, "/tests/")
}

func parsePackageFiles(t *testing.T, fset *token.FileSet, pkg listedPackage) []*ast.File {
	t.Helper()
	filenames := append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...)
	files := make([]*ast.File, 0, len(filenames))
	for _, name := range filenames {
		filename := filepath.Join(pkg.Dir, name)
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		files = append(files, file)
	}
	return files
}

func checkExportedDeclarations(t *testing.T, fset *token.FileSet, files []*ast.File, info *types.Info) {
	t.Helper()
	check := func(label string, pos token.Pos, typ types.Type) {
		if internalPath := findInternalType(typ); internalPath != "" {
			t.Errorf("%s: %s references internal package %s", fset.Position(pos), label, internalPath)
		}
	}
	checkExpr := func(label string, expr ast.Expr) {
		check(label, expr.Pos(), info.TypeOf(expr))
	}

	for _, file := range files {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Name.IsExported() {
					check("exported function or method "+decl.Name.Name, decl.Name.Pos(), info.Defs[decl.Name].Type())
				}
			case *ast.GenDecl:
				for _, rawSpec := range decl.Specs {
					switch spec := rawSpec.(type) {
					case *ast.TypeSpec:
						if !spec.Name.IsExported() {
							continue
						}
						if spec.TypeParams != nil {
							for _, field := range spec.TypeParams.List {
								checkExpr("type parameters of "+spec.Name.Name, field.Type)
							}
						}
						structure, ok := spec.Type.(*ast.StructType)
						if !ok {
							checkExpr("exported type "+spec.Name.Name, spec.Type)
							continue
						}
						for _, field := range structure.Fields.List {
							if len(field.Names) == 0 {
								checkExpr("embedded field of "+spec.Name.Name, field.Type)
								continue
							}
							for _, name := range field.Names {
								if name.IsExported() {
									checkExpr("exported field "+spec.Name.Name+"."+name.Name, field.Type)
								}
							}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								check("exported value "+name.Name, name.Pos(), info.Defs[name].Type())
							}
						}
					}
				}
			}
		}
	}
}

func findInternalType(typ types.Type) string {
	seen := make(map[types.Type]bool)
	var visit func(types.Type) string
	visitList := func(list *types.TypeList) string {
		for i := range list.Len() {
			if internalPath := visit(list.At(i)); internalPath != "" {
				return internalPath
			}
		}
		return ""
	}
	visitTuple := func(tuple *types.Tuple) string {
		for i := range tuple.Len() {
			if internalPath := visit(tuple.At(i).Type()); internalPath != "" {
				return internalPath
			}
		}
		return ""
	}
	visitTypeParams := func(list *types.TypeParamList) string {
		for i := range list.Len() {
			if internalPath := visit(list.At(i).Constraint()); internalPath != "" {
				return internalPath
			}
		}
		return ""
	}

	visit = func(current types.Type) string {
		if current == nil || seen[current] {
			return ""
		}
		seen[current] = true
		switch current := current.(type) {
		case *types.Alias:
			if isInternalObject(current.Obj()) {
				return current.Obj().Pkg().Path()
			}
			if internalPath := visitList(current.TypeArgs()); internalPath != "" {
				return internalPath
			}
			return visit(current.Rhs())
		case *types.Named:
			if isInternalObject(current.Obj()) {
				return current.Obj().Pkg().Path()
			}
			return visitList(current.TypeArgs())
		case *types.Pointer:
			return visit(current.Elem())
		case *types.Array:
			return visit(current.Elem())
		case *types.Slice:
			return visit(current.Elem())
		case *types.Map:
			if internalPath := visit(current.Key()); internalPath != "" {
				return internalPath
			}
			return visit(current.Elem())
		case *types.Chan:
			return visit(current.Elem())
		case *types.Signature:
			if internalPath := visitTypeParams(current.TypeParams()); internalPath != "" {
				return internalPath
			}
			if internalPath := visitTuple(current.Params()); internalPath != "" {
				return internalPath
			}
			return visitTuple(current.Results())
		case *types.Struct:
			for i := range current.NumFields() {
				field := current.Field(i)
				if field.Exported() || field.Embedded() {
					if internalPath := visit(field.Type()); internalPath != "" {
						return internalPath
					}
				}
			}
		case *types.Interface:
			for i := range current.NumExplicitMethods() {
				if internalPath := visit(current.ExplicitMethod(i).Type()); internalPath != "" {
					return internalPath
				}
			}
			for i := range current.NumEmbeddeds() {
				if internalPath := visit(current.EmbeddedType(i)); internalPath != "" {
					return internalPath
				}
			}
		case *types.TypeParam:
			return visit(current.Constraint())
		case *types.Union:
			for i := range current.Len() {
				if internalPath := visit(current.Term(i).Type()); internalPath != "" {
					return internalPath
				}
			}
		}
		return ""
	}
	return visit(typ)
}

func isInternalObject(obj *types.TypeName) bool {
	return obj != nil && obj.Pkg() != nil && isModuleInternalPath(obj.Pkg().Path())
}

func isModuleInternalPath(importPath string) bool {
	relative := strings.TrimPrefix(importPath, modulePath+"/")
	if relative == importPath {
		return false
	}
	for _, segment := range strings.Split(relative, "/") {
		if segment == "internal" {
			return true
		}
	}
	return false
}

func isClientSelector(expr ast.Expr, field string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != field {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == "c"
}
