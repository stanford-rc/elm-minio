package main

import (
	"go/ast"
)

// declareNames searches through an ast.GenDecl for an ast.ValueSpec declaring
// the specified name.  It will return true if it finds it, otherwise false.
func declaresName(gd *ast.GenDecl, name string) bool {
	for _, sp := range gd.Specs {
		av, ok := sp.(*ast.ValueSpec)
		if ok {
			if len(av.Names) > 0 && av.Names[0].Name == name {
				return true
			}
		}
	}
	return false
}

// extractIdents traverses the *ast.File extracts all of the *ast.Ident it
// finds under file.Decls.
func extractIdents(file *ast.File) []*ast.Ident {
	var idents []*ast.Ident

	for _, x := range file.Decls {
		gd, ok := x.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, sp := range gd.Specs {
			av, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}

			idents = append(idents, av.Names...)
		}
	}

	return idents
}
