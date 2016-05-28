package main

import (
	"os"
	"path/filepath"
)

type fileInfo struct {
	path string
	os.FileInfo
}

type walkFunc func(<-chan struct{}, <-chan fileInfo, chan<- error)

func walkAll(paths []string, walkFn filepath.WalkFunc) error {
	for _, path := range paths {
		if err := walk(path, walkFn); err != nil {
			return err
		}
	}
	return nil
}

func walk(path string, walkFn filepath.WalkFunc) error {
	var (
		stack   []*fileInfo
		current *fileInfo
	)

	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return walkFn(path, info, err)
	}

	stack = append(stack, &fileInfo{path, info})

	for pos := len(stack) - 1; pos > -1; pos = len(stack) - 1 {
		current, stack = stack[pos], stack[:pos]

		if err := walkFn(current.path, current, nil); err != nil {
			if err != filepath.SkipDir {
				return err
			}
			continue
		}

		infos, _ := readdir(current.path)

		for _, info := range infos {
			sub := filepath.Join(current.path, info.Name())

			if info.IsDir() {
				stack = append(stack, &fileInfo{sub, info})
			} else if err := walkFn(sub, info, nil); err != nil && err != filepath.SkipDir {
				return err
			}
		}
	}

	return nil
}

func readdir(path string) ([]os.FileInfo, error) {
	f, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer f.Close()

	return f.Readdir(-1)
}
