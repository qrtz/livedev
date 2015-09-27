package main

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

type Package struct {
	Context *build.Context
	Name    string
	Dir     string
}

func newPackage(context *build.Context, name, dir string) *Package {
	return &Package{context, name, dir}
}

// Import loads and return the build packagee
func (p *Package) Import() (*build.Package, error) {
	if p.Name != "" {
		return p.Context.Import(p.Name, p.Dir, build.AllowBinary)
	}

	return p.Context.ImportDir(p.Dir, build.AllowBinary)
}

// ComputeDep returns the list of the target's dependency files
func ComputeDep(context *build.Context, target string) ([]string, error) {
	var (
		queue []*Package
		files []string
	)

	info, err := os.Stat(target)

	if err != nil {
		return files, err
	}

	visited := make(map[string]bool)

	if info.IsDir() {
		queue = append(queue, newPackage(context, "", target))
	} else {
		f, err := parser.ParseFile(token.NewFileSet(), target, nil, parser.ImportsOnly)

		if err != nil {
			return files, err
		}

		d := newPackage(context, "", filepath.Dir(target))

		if f.Name != nil {
			if n := f.Name.String(); n != "main" {
				d.Name = n
				visited[d.Name] = true
			}
		}

		queue = append(queue, d)
	}

	var current *Package

	for len(queue) > 0 {

		current, queue = queue[0], queue[1:]
		// Ignore import errors. They should be caught at build time
		pkg, _ := current.Import()

		if !pkg.Goroot {
			if len(pkg.PkgObj) > 0 && fileExists(pkg.PkgObj) {
				files = append(files, pkg.PkgObj)
			}

			for _, i := range pkg.Imports {
				if !visited[i] {
					visited[i] = true
					queue = append(queue, newPackage(context, i, ""))
				}
			}

			f := addPrefix(pkg.Dir, pkg.CFiles, pkg.CgoFiles, pkg.GoFiles, pkg.HFiles, pkg.SFiles, pkg.SysoFiles)
			files = append(files, f...)
		}
	}

	return files, nil
}

// addPrefix adds prefix at beginning of each name in the list
func addPrefix(prefix string, names ...[]string) []string {
	var files []string
	for _, name := range names {
		for _, n := range name {
			if len(n) > 0 {
				files = append(files, filepath.Join(prefix, n))
			}
		}
	}

	return files
}
