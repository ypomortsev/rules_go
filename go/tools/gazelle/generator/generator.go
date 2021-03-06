/* Copyright 2016 The Bazel Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package generator provides core functionality of
// BUILD file generation in gazelle.
package generator

import (
	"log"
	"path/filepath"
	"sort"

	bzl "github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/rules_go/go/tools/gazelle/config"
	"github.com/bazelbuild/rules_go/go/tools/gazelle/packages"
	"github.com/bazelbuild/rules_go/go/tools/gazelle/rules"
)

const (
	// goRulesBzl is the label of the Skylark file which provides Go rules
	goRulesBzl = "@io_bazel_rules_go//go:def.bzl"
)

// Generator generates BUILD files for a Go repository.
type Generator struct {
	c *config.Config
	g rules.Generator
}

func New(c *config.Config) *Generator {
	return &Generator{
		c: c,
		g: rules.NewGenerator(c),
	}
}

// Generate generates a BUILD file for each Go package found under
// the given directory.
// "dir" must be an absolute path to the repository root or a subdirectory.
// Errors will be logged. BUILD files may or may not be returned for directories
// that have errors, depending on the severity of the error.
func (g *Generator) Generate(dir string) []*bzl.File {
	var files []*bzl.File
	haveTopFile := false
	packages.Walk(g.c, dir, func(pkg *packages.Package) {
		rel, err := filepath.Rel(g.c.RepoRoot, pkg.Dir)
		if err != nil {
			log.Print(err)
			return
		}
		if rel == "." {
			rel = ""
			haveTopFile = true
		}

		files = append(files, g.generateOne(rel, pkg))
	})

	if !haveTopFile {
		// The top directory of the repository did not contain buildable go
		// files, but we still need a BUILD file for go_prefix.
		// TODO: don't generate this file unless Gazelle is actually run on
		// this directory.
		files = append(files, g.emptyToplevel())
	}

	return files
}

func (g *Generator) emptyToplevel() *bzl.File {
	return &bzl.File{
		Path: g.c.DefaultBuildFileName(),
		Stmt: []bzl.Expr{
			loadExpr("go_prefix"),
			&bzl.CallExpr{
				X: &bzl.LiteralExpr{Token: "go_prefix"},
				List: []bzl.Expr{
					&bzl.StringExpr{Value: g.c.GoPrefix},
				},
			},
		},
	}
}

func (g *Generator) generateOne(rel string, pkg *packages.Package) *bzl.File {
	rs := g.g.Generate(filepath.ToSlash(rel), pkg)
	file := &bzl.File{Path: filepath.Join(rel, g.c.DefaultBuildFileName())}
	for _, r := range rs {
		file.Stmt = append(file.Stmt, r.Call)
	}
	if load := g.generateLoad(file); load != nil {
		file.Stmt = append([]bzl.Expr{load}, file.Stmt...)
	}
	return file
}

func (g *Generator) generateLoad(f *bzl.File) bzl.Expr {
	var list []string
	for _, kind := range []string{
		"go_prefix",
		"go_library",
		"go_binary",
		"go_test",
		"cgo_library",
	} {
		if len(f.Rules(kind)) > 0 {
			list = append(list, kind)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return loadExpr(list...)
}

func loadExpr(rules ...string) *bzl.CallExpr {
	sort.Strings(rules)

	list := []bzl.Expr{
		&bzl.StringExpr{Value: goRulesBzl},
	}
	for _, r := range rules {
		list = append(list, &bzl.StringExpr{Value: r})
	}

	return &bzl.CallExpr{
		X:            &bzl.LiteralExpr{Token: "load"},
		List:         list,
		ForceCompact: true,
	}
}
