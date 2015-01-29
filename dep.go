package main

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

type Dep struct {
	Context *build.Context
	Name    string
	Dir     string
}

func newDep(context *build.Context, name, dir string) *Dep {
	return &Dep{context, name, dir}
}

func (p *Dep) Import() (*build.Package, error) {
	if p.Name != "" {
		return p.Context.Import(p.Name, p.Dir, build.AllowBinary)
	}

	return p.Context.ImportDir(p.Dir, build.AllowBinary)
}

//Computes the target's dependencies
func ComputeDep(context *build.Context, target string) ([]string, error) {
	var (
		queue []*Dep
		files []string
	)

	info, err := os.Stat(target)

	if err != nil {
		return files, err
	}

	visited := make(map[string]bool)

	if info.IsDir() {
		queue = append(queue, newDep(context, "", target))
	} else {
		f, err := parser.ParseFile(token.NewFileSet(), target, nil, parser.ImportsOnly)

		if err != nil {
			return files, err
		}

		d := newDep(context, "", filepath.Dir(target))

		if f.Name != nil {
			if n := f.Name.String(); n != "main" {
				d.Name = n
				visited[d.Name] = true
			}
		}

		queue = append(queue, d)
	}

	var current *Dep

	for len(queue) > 0 {

		current, queue = queue[0], queue[1:]
		//Ignore import errors. They should be caught a build time
		pkg, _ := current.Import()

		if !pkg.Goroot {
			if len(pkg.PkgObj) > 0 && fileExists(pkg.PkgObj) {
				files = append(files, pkg.PkgObj)
			}

			for _, i := range pkg.Imports {
				if !visited[i] {
					visited[i] = true
					queue = append(queue, newDep(context, i, ""))
				}
			}

			f := concat(pkg.Dir, pkg.CFiles, pkg.CgoFiles, pkg.GoFiles, pkg.HFiles, pkg.SFiles, pkg.SysoFiles)
			files = append(files, f...)
		}
	}

	return files, nil
}

func concat(path string, elements ...[]string) []string {
	var files []string
	for _, e := range elements {
		for _, v := range e {
			if len(v) > 0 {
				files = append(files, filepath.Join(path, v))
			}
		}
	}

	return files
}
