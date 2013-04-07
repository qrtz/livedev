package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
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

func readdir(path string) ([]os.FileInfo, error) {
	f, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer f.Close()

	return f.Readdir(-1)
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

		if infos, err := readdir(current.Path); err != nil {
			return err
		} else {

			for _, info := range infos {
				sub := filepath.Join(current.Path, info.Name())

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

type Done struct {
	Path  string
	mtime time.Time
	Err   error
}

func ModTimeList(paths []string, skip *regexp.Regexp) (mtime time.Time, err error) {
	wg := new(sync.WaitGroup)
	ln := 8

	if n := len(paths); n < ln {
		ln = n
	}

	ch := make(chan Done, ln)

	for _, p := range paths {
		wg.Add(1)
		go func(c chan Done, path string, skip *regexp.Regexp) {
			defer wg.Done()
			mt, err := ModTime(path, skip)
			c <- Done{path, mt, err}
		}(ch, p, skip)
	}

	go func(c chan Done, w *sync.WaitGroup) {
		w.Wait()
		close(c)
	}(ch, wg)

	for d := range ch {
		if d.Err != nil && d.Err != ERR_SKIP {
			err = d.Err
			return
		}

		if d.mtime.After(mtime) {
			mtime = d.mtime
		}
	}
	return
}

func FileModTime(dirs []string, skip *regexp.Regexp) (mtime time.Time, err error) {
	wg := new(sync.WaitGroup)
	ch := make(chan Done, len(dirs))

	for _, d := range dirs {
		wg.Add(1)
		go func(c chan Done, dir string, skip *regexp.Regexp) {
			defer wg.Done()
			mt, err := ModTime(dir, skip)
			c <- Done{dir, mt, err}
		}(ch, d, skip)
	}

	go func(c chan Done, w *sync.WaitGroup) {
		w.Wait()
		close(c)
	}(ch, wg)

	for d := range ch {
		if d.Err != nil && d.Err != ERR_SKIP {
			err = d.Err
			return
		}

		if d.mtime.After(mtime) {
			mtime = d.mtime
		}
	}
	return
}

func ModTime(dir string, skip *regexp.Regexp) (mtime time.Time, err error) {
	err = Traverse(dir, func(name string, info os.FileInfo, fileErr error) error {

		if (fileErr != nil && fileErr != ERR_NOT_DIR) || (skip != nil && skip.MatchString(name)) {
			return ERR_SKIP
		}

		if !info.IsDir() {
			t := info.ModTime()
			if t.After(mtime) {
				mtime = t
			}
		}
		return nil
	})

	return
}
