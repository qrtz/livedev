package main

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

type pkg struct {
	Context *build.Context
	Name    string
	Dir     string
}

func newPackage(context *build.Context, name, dir string) *pkg {
	return &pkg{context, name, dir}
}

// Import loads and return the build packagee
func (p *pkg) Import() (*build.Package, error) {
	if p.Name != "" {
		return p.Context.Import(p.Name, p.Dir, build.AllowBinary)
	}

	return p.Context.ImportDir(p.Dir, build.AllowBinary)
}

// computeDep returns the list of the target's dependency files
func computeDep(context *build.Context, target string) ([]string, error) {
	var (
		queue []*pkg
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

	var current *pkg

	for len(queue) > 0 {

		current, queue = queue[0], queue[1:]
		// Ignore import errors. They should be caught at build time
		p, _ := current.Import()

		if !p.Goroot {
			if len(p.PkgObj) > 0 && fileExists(p.PkgObj) {
				files = append(files, p.PkgObj)
			}

			for _, i := range p.Imports {
				if !visited[i] {
					visited[i] = true
					queue = append(queue, newPackage(context, i, ""))
				}
			}

			f := addPrefix(p.Dir, p.CFiles, p.CgoFiles, p.GoFiles, p.HFiles, p.SFiles, p.SysoFiles)
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
