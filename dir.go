package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ERR_SKIP    = errors.New("SKIP")
	ERR_NOT_DIR = errors.New("Not a directory")
)

type File struct {
	Path string
	os.FileInfo
}

type FileVisitorFunc func(string, os.FileInfo, error) error

//TODO: Concurrency
func TraverseList(paths []string, visit FileVisitorFunc) error {
	for _, p := range paths {
		if err := Traverse(p, visit); err != nil {
			return err
		}
	}

	return nil
}

func readdir(path string) ([]string, error) {
	f, err := os.Open(path)

	if err != nil {
		return []string{}, err
	}
	
	defer f.Close()

	return f.Readdirnames(-1)
}

func Traverse(path string, visit FileVisitorFunc) error {
	var (
		stack   []*File
		current *File
	)

	if info, err := os.Lstat(path); err != nil {
		return visit(path, info, err)
	} else if !info.IsDir() {
		return visit(path, info, ERR_NOT_DIR)
	} else {
		stack = append(stack, &File{path, info})
	}

	for pos := len(stack) - 1; pos > -1; pos = len(stack) - 1 {
		current, stack = stack[pos], stack[:pos]

		if err := visit(current.Path, current, nil); err != nil {
			if err == ERR_SKIP {
				continue
			}
			return err
		}

		names, err := readdir(current.Path)

		if err != nil {
			return err
		}

		for _, name := range names {
			sub := filepath.Join(current.Path, name)
			info, err := os.Lstat(sub)

			if err != nil {
				if err := visit(sub, info, err); err != nil && err != ERR_SKIP {
					return err
				}
			} else {

				if info.IsDir() {
					stack = append(stack, &File{sub, info})
				} else {
					if err := visit(sub, info, nil); err != nil && err != ERR_SKIP {
						return err
					}
				}
			}
		}
	}

	return nil
}

func ModTimeList(dirs []string, skip *regexp.Regexp) (buildFileTime, appFileTime time.Time, err error) {
	for _, d := range dirs {
		b, a, e := ModTime(d, skip)
		if e != nil {
			err = e
			return
		}

		if a.After(appFileTime) {
			appFileTime = a
		}

		if b.After(buildFileTime) {
			buildFileTime = b
		}
	}
	return
}

func ModTime(dir string, skip *regexp.Regexp) (buildFileTime, appFileTime time.Time, err error) {
	err = Traverse(dir, func(name string, info os.FileInfo, fileErr error) error {

		if fileErr != nil || (skip != nil && skip.MatchString(name)) {
			return ERR_SKIP
		}

		if !info.IsDir() {
			t := info.ModTime()
			if strings.HasSuffix(name, ".go") {
				if t.After(buildFileTime) {
					buildFileTime = t
				}
			} else if t.After(appFileTime) {
				appFileTime = t
			}
		}
		return nil
	})

	return
}
