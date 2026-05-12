package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"

	"golang.org/x/tools/go/ast/astutil"
)

// patch minio-pkg/policy/condition/keyname.go to add additional JWT Claims
type patchPkgJWTClaims struct {
	// addl_claims lists one or more KeyName const that should be added to
	// the target source file
	addl_claims string

	// extend_keyname_slices lists the slices that should have the
	// addl_claims KeyName added to them
	extend_keyname_slices map[string]bool

	// jwt_const_member_id is the name of a variable in the source that we
	// want to insert addl_claims to just after
	jwt_const_member_id string
}

func PatchPkgJWTClaims() *patchPkgJWTClaims {
	return &patchPkgJWTClaims{
		addl_claims: `
			const (
				JWTEduPersonEntitlement KeyName = "jwt:edupersonentitlement"
			)
		`,

		extend_keyname_slices: map[string]bool{
			"JWTKeys":          true,
			"AllSupportedKeys": true,
		},

		jwt_const_member_id: "JWTGroups",
	}
}

// The  OIDC service transmits important claim details using a claim named
// edupersonentitlement [1], but MinIO does not support referencing that claim in
// its dynamic policy documents [2]. Support in the policy documents for all
// referenced claims names is a requirement if we want to use the more flexible
// RolePolicy flow authentication scheme [3], with its dependency on the MinIO STS
// AssumeRoleWithWebIdentity service [4].
//
// Accordingly, we want to update the keyname.go source code to include the
// additional claim names that we need supported.  The policy.patch program uses
// the go AST libraries to parse the keyname.go source code and modify it to add
// the custom claims.  Optionally policy.patch can then update the source code in
// place.  Otherwise, by default, it will print the updated code to stdout.
//
// An alternative to this program would be to make the required changes and record
// a diff for patch(1).  This program was developed in the hope that it would
// prove to be more robust than patching, as it only needs to rely on a couple of
// keywords remaining unchanged in the keyname.go source code.
//
// The policy.patch tool is expecting that the keyname.go source code contains the
// basic structure:
//
//	type KeyName string
//	...
//	const (
//	    ...
//	    JWTGroups ...
//	    ...
//	)
//	...
//	var JWTKeys = []KeyName{
//	    ...
//	}
//	...
//	var AllSupportedKeys = []KeyName{
//	    ...
//	}
//
// It will add a new const group below the JWTGroups set, adding the new claims we
// want to support, and will extend the JWTKeys slice, listing the new claims:
//
//   - diff -u /build/src/minio-pkg/policy/condition/keyname.go.orig /build/src/minio-pkg/policy/condition/keyname.go
//     --- /build/src/minio-pkg/policy/condition/keyname.go.orig   2024-02-13 00:01:54.953013789 +0000
//     +++ /build/src/minio-pkg/policy/condition/keyname.go        2024-02-13 00:01:54.953013789 +0000
//     @@ -193,6 +193,9 @@
//     JWTScope        KeyName = "jwt:scope"
//     JWTClientID     KeyName = "jwt:client_id"
//     )
//     +const (
//
//   - JWTEduPersonEntitlement KeyName = "jwt:edupersonentitlement"
//     +)
//
//     const (
//     // LDAPUser - LDAP username, in MinIO this value is equal to your authenticating LDAP user DN.
//     @@ -236,7 +239,7 @@
//     JWTPhoneNumber,
//     JWTAddress,
//     JWTScope,
//
//   - JWTClientID,
//
//   - JWTClientID, JWTEduPersonEntitlement,
//     }
//
//     // AllSupportedKeys - is list of all all supported keys.
//     @@ -298,7 +301,7 @@
//     JWTScope,
//     JWTClientID,
//     STSDurationSeconds,
//
//   - SVCDurationSeconds,
//
//   - SVCDurationSeconds, JWTEduPersonEntitlement,
//     }
//
//     // CommonKeys - is list of all common condition keys.
//     +
//
//     REFERENCES
//
// [1] https://uit.stanford.edu/service/oidc/scope-claims
// [2] https://github.com/minio/minio/blob/master/docs/multi-user/README.md#policy-variables
// [3] https://min.io/docs/minio/linux/administration/identity-access-management/oidc-access-management.html#minio-external-identity-management-openid
// [4] https://min.io/docs/minio/linux/developers/security-token-service/AssumeRoleWithWebIdentity.html#minio-sts-assumerolewithwebidentity
func (patch *patchPkgJWTClaims) Patch(fpath string) (*bytes.Buffer, error) {
	// addlClaims returns an *ast.File for a minimalist source file containing
	// const declarations for additional JWT claims to add to the
	// github.com/stanford-rc/minio-pkg keyname.go source code.
	addlClaims := func(fname, pkgname string) (*ast.File, error) {
		src := fmt.Sprintf("package %s\n%s\n", pkgname, patch.addl_claims)
		return parser.ParseFile(token.NewFileSet(), fname, src, parser.AllErrors)
	}

	// appendIdentDecl returns an astutil.ApplyFunc that will append the
	// provided idents to the first <jwt_keys_var_id> declaration it finds.
	// If the entry is not the expected type of *ast.CompositeLit a fatal
	// error is printed and the program will exit.
	appendIdentDecl := func(idents []*ast.Ident) func(*astutil.Cursor) bool {
		return func(c *astutil.Cursor) bool {
			switch n := c.Node().(type) {
			case *ast.GenDecl:
				// only inspect var declarations
				if n.Tok != token.VAR {
					return true
				}

				// Inspect each *ast.ValueSpec and look for one
				// matching extend_keyname_slices keys.  If it is a
				// slice, extend it with our new KeyName.
				//
				// Note that minio/pkg v2.0.8 (git commit 4a6e501de321)
				// set AllSupportedKeys using a call expression that
				// was partially derived from JWTKeys.
				//
				// v2.0.9 (git commit 7990a27fd79) changed
				// AllSupportedKeys to be set as a simple slice
				// declaration that explicitly duplicates the entries
				// in JWTKeys.
				for _, sp := range n.Specs {
					av, ok := sp.(*ast.ValueSpec)
					if !ok {
						continue
					}

					for i, avn := range av.Names {
						if !patch.extend_keyname_slices[avn.Name] {
							continue
						}

						if cp, ok := av.Values[i].(*ast.CompositeLit); ok {
							for _, kn := range idents {
								cp.Elts = append(cp.Elts, kn)
							}
						}
					}
				}
			}

			return true
		}
	}

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

	// generate additional JWT claims that we want to add to the AST
	addlClaim, err := addlClaims("jwt_claims_stanford.go", file.Name.Name)
	if err != nil {
		return nil, fmt.Errorf("error generating addlClaim: %v", err)
	}

	// walk the updated source AST and search for the <jwt_keys_var_id>
	// declaration, which we expect to be a *ast.CompositeLit, and adding
	// our new *ast.Ident from addlClaim to the end of the list.
	addlIdents := extractIdents(addlClaim)
	updated, ok := astutil.Apply(file, nil, appendIdentDecl(addlIdents)).(*ast.File)
	if !ok {
		return nil, fmt.Errorf("asutil.Apply did not return the expected *ast.File: %#v", updated)
	}

	// iterate over the updated AST's top level declarations and look for
	// the first const group containing <jwt_const_member_id>.  Once we've
	// identified that declaration insert our addlClaim.Decls just after
	// it.
	for i, x := range updated.Decls {
		gd, ok := x.(*ast.GenDecl)
		if !ok {
			continue
		}

		if gd.Tok != token.CONST {
			continue
		}

		if declaresName(gd, patch.jwt_const_member_id) {
			if i == len(file.Decls)-1 {
				file.Decls = append(file.Decls, addlClaim.Decls...)
			} else {
				var decls []ast.Decl
				decls = append(decls, file.Decls[:i+1]...)
				decls = append(decls, addlClaim.Decls...)
				file.Decls = append(decls, file.Decls[i+1:]...)
			}
			break
		}
	}

	// return the updated source code
	buf := &bytes.Buffer{}
	format.Node(buf, fset, updated)

	return buf, nil
}
