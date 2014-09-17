package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	ErrModified = errors.New("modified")
)

type FileInfo struct {
	Path string
	os.FileInfo
}

type WalkFunc func(<-chan struct{}, <-chan FileInfo, chan<- error)

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
		stack   []*FileInfo
		current *FileInfo
	)

	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return walkFn(path, info, err)
	}

	stack = append(stack, &FileInfo{path, info})

	for pos := len(stack) - 1; pos > -1; pos = len(stack) - 1 {
		current, stack = stack[pos], stack[:pos]

		if err := walkFn(current.Path, current, nil); err != nil {
			if err != filepath.SkipDir {
				return err
			}
			continue
		}

		infos, _ := readdir(current.Path)

		for _, info := range infos {
			sub := filepath.Join(current.Path, info.Name())

			if info.IsDir() {
				stack = append(stack, &FileInfo{sub, info})
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

func ModifiedSince(since time.Time, ignore *regexp.Regexp, files ...string) (bool, error) {
	err := walkAll(files, func(path string, info os.FileInfo, err error) error {
		if ignore != nil && ignore.MatchString(info.Name()) {
			if info.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if info.ModTime().After(since) {
			return ErrModified
		}

		return nil
	})

	if err == filepath.SkipDir {
		return false, nil
	}

	if err == ErrModified {
		return true, nil
	}

	return false, err
}
