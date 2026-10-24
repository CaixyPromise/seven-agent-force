// Source-only documentation extractor. It never starts the application or reads configuration.
// Run from seven-agent-force-server; see ../README.md for the export-index command.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/CaixyPromise/seven-agent/force/"

type pkgIndex struct {
	ImportPath, Dir, Export string
	GoFiles                 []string
}
type field struct {
	Name, GoType, Tag, Reference string
	Embedded                     bool
}
type shape struct {
	Name, Kind, Underlying, Source string
	Fields                         []field
	Enum                           []string
}
type parameter struct{ In, Name, Type string }
type routeDoc struct {
	Method, Path, Handler, Source, Registration, Permission string
	Parameters                                              []parameter
	Requests, Responses, ResponseExpressions, ErrorCodes    []string
}
type fn struct {
	node   *ast.FuncDecl
	info   *types.Info
	source string
	pkg    string
}
type extractor struct {
	fs        *token.FileSet
	functions map[*types.Func]fn
	shapes    map[string]shape
	routes    []routeDoc
	info      map[string]*types.Info
	files     map[string][]*ast.File
}

func name(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return strings.TrimPrefix(p.Path(), module) })
}
func literal(e ast.Expr) string {
	if v, ok := e.(*ast.BasicLit); ok && v.Kind == token.STRING {
		s, _ := strconv.Unquote(v.Value)
		return s
	}
	return ""
}
func (x *extractor) source(p token.Pos) string {
	v := x.fs.Position(p)
	cwd, _ := os.Getwd()
	r, err := filepath.Rel(cwd, v.Filename)
	if err != nil {
		return ""
	}
	return "seven-agent-force-server/" + filepath.ToSlash(r) + ":" + strconv.Itoa(v.Line)
}
func shapeName(t types.Type) string {
	if _, ok := t.(*types.Struct); ok {
		return fmt.Sprintf("inline.%x", sha256.Sum256([]byte(name(t))))[:19]
	}
	return name(t)
}
func (x *extractor) ref(t types.Type) string {
	if t == nil {
		return ""
	}
	if a, ok := t.(*types.Alias); ok {
		return x.ref(types.Unalias(a))
	}
	switch v := t.(type) {
	case *types.Pointer:
		return x.ref(v.Elem())
	case *types.Slice:
		return x.ref(v.Elem())
	case *types.Array:
		return x.ref(v.Elem())
	case *types.Map:
		return x.ref(v.Elem())
	case *types.Named:
		if v.Obj().Pkg() != nil && strings.HasPrefix(v.Obj().Pkg().Path(), module) {
			return name(v)
		}
	case *types.Struct:
		return shapeName(v)
	}
	return ""
}
func (x *extractor) shape(t types.Type) {
	if t == nil {
		return
	}
	if a, ok := t.(*types.Alias); ok {
		x.shape(types.Unalias(a))
		return
	}
	switch v := t.(type) {
	case *types.Pointer:
		x.shape(v.Elem())
		return
	case *types.Slice:
		x.shape(v.Elem())
		return
	case *types.Array:
		x.shape(v.Elem())
		return
	case *types.Map:
		x.shape(v.Elem())
		return
	}
	n, named := t.(*types.Named)
	if named && (n.Obj().Pkg() == nil || !strings.HasPrefix(n.Obj().Pkg().Path(), module)) {
		return
	}
	if _, ok := t.(*types.Struct); !named && !ok {
		return
	}
	k := shapeName(t)
	if _, ok := x.shapes[k]; ok {
		return
	}
	s := shape{Name: k, Underlying: name(t.Underlying())}
	if named {
		s.Source = x.source(n.Obj().Pos())
	}
	x.shapes[k] = s
	if st, ok := t.Underlying().(*types.Struct); ok {
		s.Kind = "object"
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			tag := st.Tag(i)
			j := reflect.StructTag(tag).Get("json")
			if (!f.Exported() && !f.Anonymous()) || j == "-" {
				continue
			}
			key := strings.Split(j, ",")[0]
			if key == "" {
				key = f.Name()
			}
			s.Fields = append(s.Fields, field{Name: key, GoType: name(f.Type()), Tag: tag, Reference: x.ref(f.Type()), Embedded: f.Anonymous() && j == ""})
			x.shape(f.Type())
		}
	} else {
		s.Kind = "value"
	}
	if named {
		for _, key := range n.Obj().Pkg().Scope().Names() {
			o := n.Obj().Pkg().Scope().Lookup(key)
			if c, ok := o.(*types.Const); ok && types.Identical(c.Type(), n) {
				s.Enum = append(s.Enum, c.Val().ExactString())
			}
		}
	}
	x.shapes[k] = s
}
func function(e ast.Expr, info *types.Info) *types.Func {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		if s := info.Selections[v]; s != nil {
			f, _ := s.Obj().(*types.Func)
			return f
		}
		f, _ := info.Uses[v.Sel].(*types.Func)
		return f
	case *ast.Ident:
		f, _ := info.Uses[v].(*types.Func)
		return f
	}
	return nil
}
func handler(e ast.Expr, info *types.Info) *types.Func {
	if f := function(e, info); f != nil {
		return f
	}
	if c, ok := e.(*ast.CallExpr); ok {
		for i := len(c.Args) - 1; i >= 0; i-- {
			if f := handler(c.Args[i], info); f != nil {
				return f
			}
		}
		return function(c.Fun, info)
	}
	return nil
}
func callee(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}
func unique(in []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	return out
}
func (x *extractor) inspect(f fn, d *routeDoc, depth int, seen map[*ast.FuncDecl]bool) {
	if f.node == nil || seen[f.node] || depth > 4 {
		return
	}
	seen[f.node] = true
	ast.Inspect(f.node.Body, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		method := callee(c.Fun)
		if method == "Query" || method == "DefaultQuery" || method == "PostForm" || method == "FormValue" || method == "FormFile" || method == "Peek" {
			if len(c.Args) > 0 && literal(c.Args[0]) != "" {
				where := "query"
				if method == "Peek" {
					where = "header"
				}
				if method == "PostForm" || method == "FormValue" {
					where = "form"
				}
				if method == "FormFile" {
					where = "file"
				}
				typ := "string"
				if where == "file" {
					typ = "binary multipart file"
				}
				d.Parameters = append(d.Parameters, parameter{where, literal(c.Args[0]), typ})
			}
		}
		if method == "Decode" || method == "Unmarshal" || method == "BindAndValidate" || method == "BindJSON" || strings.HasPrefix(method, "decode") {
			for _, a := range c.Args {
				if u, ok := a.(*ast.UnaryExpr); ok && u.Op == token.AND {
					t := f.info.TypeOf(u.X)
					if t != nil && name(t) != "any" && name(t) != "struct{}" {
						d.Requests = append(d.Requests, shapeName(t))
						x.shape(t)
					}
				}
			}
		}
		if (method == "Success" || method == "SuccessWithStatus" || (method == "write" || method == "writeResult" || method == "writeOneTimeSecret")) && len(c.Args) > 1 {
			a := c.Args[len(c.Args)-1]
			if method == "write" || method == "writeResult" || method == "writeOneTimeSecret" {
				a = c.Args[1]
			}
			t := f.info.TypeOf(a)
			if t != nil && name(t) != "any" {
				d.Responses = append(d.Responses, shapeName(t))
				x.shape(t)
			}
			if comp, ok := a.(*ast.CompositeLit); ok {
				for _, elt := range comp.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						t := f.info.TypeOf(kv.Value)
						if t != nil {
							x.shape(t)
							d.ResponseExpressions = append(d.ResponseExpressions, literal(kv.Key)+": "+name(t))
						}
					}
				}
			}
		}
		if strings.Contains(strings.ToLower(method), "error") || method == "ErrorWithReason" {
			for _, a := range c.Args {
				s := literal(a)
				if s != "" && strings.ToUpper(s) == s && strings.Contains(s, "_") {
					d.ErrorCodes = append(d.ErrorCodes, s)
				}
			}
		}
		if g := function(c.Fun, f.info); g != nil {
			if next, ok := x.functions[g]; ok && next.pkg == f.pkg && (g.Type().(*types.Signature).Recv() == nil || !strings.HasPrefix(g.Name(), "Mount")) {
				x.inspect(next, d, depth+1, seen)
			} else if ok && next.pkg == f.pkg && next.node != f.node && strings.HasPrefix(g.Name(), "write") {
				x.inspect(next, d, depth+1, seen)
			}
		}
		return true
	})
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: extract export-index.json output.json (cwd seven-agent-force-server)")
	}
	input, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer input.Close()
	index := map[string]pkgIndex{}
	dec := json.NewDecoder(input)
	for {
		var p pkgIndex
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		index[p.ImportPath] = p
	}
	x := extractor{fs: token.NewFileSet(), functions: map[*types.Func]fn{}, shapes: map[string]shape{}, info: map[string]*types.Info{}, files: map[string][]*ast.File{}}
	imp := importer.ForCompiler(x.fs, "gc", func(path string) (io.ReadCloser, error) {
		p, ok := index[path]
		if !ok || p.Export == "" {
			return nil, fmt.Errorf("missing export: %s", path)
		}
		return os.Open(p.Export)
	})
	paths := []string{}
	for path := range index {
		if strings.HasPrefix(path, module+"internal/app/workforce/") && (strings.HasSuffix(path, "/controller") || strings.HasSuffix(path, "/domain") || strings.HasSuffix(path, "/facade")) || strings.HasPrefix(path, module+"internal/worker/") || path == module+"internal/mcpserver" || strings.HasPrefix(path, module+"cmd/seven-worker") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		p := index[path]
		if len(p.GoFiles) == 0 {
			continue
		}
		files := []*ast.File{}
		for _, file := range p.GoFiles {
			f, err := parser.ParseFile(x.fs, filepath.Join(p.Dir, file), nil, parser.ParseComments)
			if err != nil {
				panic(err)
			}
			files = append(files, f)
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
		cfg := types.Config{Importer: imp}
		pkg, err := cfg.Check(path, x.fs, files, info)
		if err != nil {
			panic(err)
		}
		x.info[path] = info
		x.files[path] = files
		for _, file := range files {
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.FuncDecl); ok {
					if o, ok := info.Defs[f.Name].(*types.Func); ok {
						x.functions[o] = fn{f, info, x.source(f.Pos()), path}
					}
				}
			}
		}
		// Local SDK wire declarations (even when not referenced by an HTTP response).
		if strings.Contains(path, "internal/worker/") || strings.Contains(path, "internal/mcpserver") || strings.Contains(path, "cmd/seven-worker") || strings.HasSuffix(path, "/domain") || strings.HasSuffix(path, "/facade") {
			for _, key := range pkg.Scope().Names() {
				if o, ok := pkg.Scope().Lookup(key).(*types.TypeName); ok {
					if s, ok := o.Type().Underlying().(*types.Struct); ok {
						tagged := false
						for i := 0; i < s.NumFields(); i++ {
							tagged = tagged || reflect.StructTag(s.Tag(i)).Get("json") != ""
						}
						if tagged {
							x.shape(o.Type())
						}
					}
				}
			}
		}
	}
	for _, path := range paths {
		if !strings.HasSuffix(path, "/controller") {
			continue
		}
		info := x.info[path]
		for _, file := range x.files[path] {
			// This legacy machine controller is not mounted by workforce.Module.
			if strings.HasSuffix(x.fs.Position(file.Pos()).Filename, "/executionaccess/controller/machine.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok || len(c.Args) < 2 {
					return true
				}
				method := callee(c.Fun)
				if !strings.Contains(" GET POST PUT PATCH DELETE HEAD OPTIONS ", " "+method+" ") {
					return true
				}
				p := literal(c.Args[0])
				if !strings.HasPrefix(p, "/") {
					return true
				}
				f := handler(c.Args[len(c.Args)-1], info)
				d := routeDoc{Method: method, Path: "/api" + p, Registration: x.source(c.Pos())}
				if f != nil {
					d.Handler = f.Name()
					if def, ok := x.functions[f]; ok {
						d.Source = def.source
						x.inspect(def, &d, 0, map[*ast.FuncDecl]bool{})
					}
				}
				ast.Inspect(c.Args[len(c.Args)-1], func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						for _, a := range call.Args {
							v := literal(a)
							if strings.HasPrefix(v, "workforce:") {
								d.Permission = v
							}
						}
					}
					return true
				})
				for _, part := range strings.Split(p, "/") {
					if strings.HasPrefix(part, ":") {
						d.Parameters = append(d.Parameters, parameter{"path", part[1:], "string"})
					}
				}
				d.Requests = unique(d.Requests)
				d.Responses = unique(d.Responses)
				d.ResponseExpressions = unique(d.ResponseExpressions)
				d.ErrorCodes = unique(d.ErrorCodes)
				seen := map[string]bool{}
				params := []parameter{}
				for _, p := range d.Parameters {
					key := p.In + ":" + p.Name
					if !seen[key] {
						seen[key] = true
						params = append(params, p)
					}
				}
				d.Parameters = params
				x.routes = append(x.routes, d)
				return true
			})
		}
	}
	sort.Slice(x.routes, func(i, j int) bool {
		if x.routes[i].Path == x.routes[j].Path {
			return x.routes[i].Method < x.routes[j].Method
		}
		return x.routes[i].Path < x.routes[j].Path
	})
	shapes := []shape{}
	for _, s := range x.shapes {
		shapes = append(shapes, s)
	}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Name < shapes[j].Name })
	f, err := os.Create(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"routes": x.routes, "types": shapes}); err != nil {
		panic(err)
	}
	fmt.Printf("routes=%d types=%d\n", len(x.routes), len(shapes))
}
