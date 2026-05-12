package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

type patchMinioGlobals struct {
	// comments and their new values
	comments map[string]string

	// global variables and their new expression values
	globals map[string]string
}

func PatchMinioGlobals() *patchMinioGlobals {
	return &patchMinioGlobals{
		comments: map[string]string{
			"// Minimum Part size for multipart upload is 5MiB": "// Minimum Part size for multipart upload is 5GiB",
		},
		globals: map[string]string{
			"globalMinPartSize": "5 * humanize.GiByte",
		},
	}
}

func (patch *patchMinioGlobals) Patch(fpath string) (*bytes.Buffer, error) {
	// read original source as fbody
	fbody, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %s: %v", fpath, err)
	}

	// parse fbody into an AST
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fpath, fbody, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("error parsing file: %s: %v", fpath, err)
	}

	// inspect the ast file and look for nodes to update
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.ValueSpec:
			if val, ok := patch.globals[n.Names[0].Name]; ok {
				if expr, err := parser.ParseExpr(val); err == nil {
					n.Values[0] = expr
				} else {
					panic(fmt.Errorf("unable to compile %s expr %s: %v",
						n.Names[0].Name, val, err))
				}
			}
		case *ast.Comment:
			if val, ok := patch.comments[n.Text]; ok {
				n.Text = val
			}
		}
		return true
	})

	// return the updated source code
	buf := &bytes.Buffer{}
	format.Node(buf, fset, file)

	return buf, nil
}
