package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Builder struct {
	lock sync.Mutex
	bin  string
}

func NewBuilder(bin string) *Builder {
	return &Builder{bin: bin}
}

func (b *Builder) Build(target string, source []string, output string) error {
	env := NewEnv(os.Environ())

	//Date the source folder to the process GOPATH
	for _, s := range source {
		env.Add(KEY_GOPATH, s)
	}
	cmd := exec.Command(b.bin, "build", "-o", output, target)
	cmd.Env = env.Data()

	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) > 0 {
			r := bufio.NewReader(bytes.NewReader(out))
			var lines []string
		done:
			for {
				line, err := r.ReadString('\n')
				switch {
				case err != nil:
					break done
				case !strings.HasPrefix(line, "#"):
					lines = append(lines, line)
				}
			}
			return errors.New(strings.Join(lines, "\n"))
		}

		return err
	}

	return nil
}
