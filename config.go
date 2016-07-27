package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/scanner"
	"time"
	"unicode"

	"errors"
	"github.com/qrtz/livedev/env"
	// "net"
	"path/filepath"
	"strconv"
	"strings"
)

var errInvalidSyntax = errors.New("Invalid syntax")

type serverConfig struct {
	Default        bool              `json:"default"`
	Host           string            `json:"host"`
	Port           int               `json:"port"`
	Bin            string            `json:"bin"`
	Resources      resourceConfig    `json:"resources"`
	Assets         resourceConfig    `json:"assets"`
	Target         string            `json:"target"`
	WorkingDir     string            `json:"workingDir"`
	Startup        []string          `json:"startup"`
	Builder        []string          `json:"builder"`
	GoRoot         string            `json:"GOROOT,omitempty"`
	GoPath         []string          `json:"GOPATH,omitempty"`
	StartupTimeout time.Duration     `json:"startupTimeout,omitempty"`
	Env            map[string]string `json:"env"`
}

func (c *serverConfig) UnmarshalJSON(data []byte) error {
	type cfg serverConfig
	var conf cfg
	err := json.Unmarshal(data, &conf)
	if err != nil {
		return err
	}

	if len(conf.Host) == 0 {
		conf.Host = "localhost"
	}

	conf.Bin = strings.TrimSpace(conf.Bin)
	conf.Target = strings.TrimSpace(conf.Target)
	conf.WorkingDir = strings.TrimSpace(conf.WorkingDir)

	if len(conf.WorkingDir) == 0 {
		conf.WorkingDir = filepath.Dir(conf.Target)
	}

	if conf.Port == 0 {
		addr, err := findAvailablePort()

		if err != nil {
			return err
		}

		conf.Port = addr.Port
	}

	if len(conf.Bin) == 0 {
		conf.Bin = filepath.Join(os.TempDir(), fmt.Sprintf("livedev-%s-%d", conf.Host, conf.Port))
	}

	*c = serverConfig(conf)
	return nil
}

type resourceConfig struct {
	Ignore string   `json:"ignore"`
	Paths  []string `json:"paths"`
}

type config struct {
	Port           int            `json:"port,omitempty"` //proxy port
	GoRoot         string         `json:"GOROOT,omitempty"`
	GoPath         []string       `json:"GOPATH"`
	Servers        []serverConfig `json:"server"`
	StartupTimeout time.Duration  `json:"startupTimeout,omitempty"`
}

func (c *config) UnmarshalJSON(data []byte) error {
	type cfg config
	var conf cfg
	err := json.Unmarshal(data, &conf)
	if err != nil {
		return err
	}

	if conf.Port == 0 {
		conf.Port = c.Port
	}

	if conf.StartupTimeout == 0 {
		conf.StartupTimeout = c.StartupTimeout
	}

	conf.GoRoot = strings.TrimSpace(conf.GoRoot)

	if len(conf.GoRoot) == 0 {
		conf.GoRoot = c.GoRoot
	}

	if len(conf.GoPath) == 0 {
		conf.GoPath = c.GoPath
	}

	for i := range conf.Servers {
		s := &conf.Servers[i]
		if len(s.GoPath) == 0 {
			s.GoPath = conf.GoPath
		}

		if len(s.GoRoot) == 0 {
			s.GoRoot = conf.GoRoot
		}

		if s.StartupTimeout == 0 {
			s.StartupTimeout = conf.StartupTimeout
		}

		if err := processConfig(s, env.New(os.Environ()), '`'); err != nil {
			return err
		}

	}

	*c = config(conf)
	return nil
}

func loadConfig(configFile string, conf *config) error {
	r, err := os.Open(configFile)

	if err != nil {
		return fmt.Errorf("Unable to read configution file: %s\n%s", configFile, err.Error())
	}

	defer r.Close()

	dec := json.NewDecoder(r)
	if err := dec.Decode(conf); err != nil {
		return fmt.Errorf("Unable to parse configution file: %s\n%s", configFile, err.Error())
	}

	return nil
}

func processConfig(conf *serverConfig, ev *env.Env, escapeChar rune) error {
	b, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	m := make(map[string]interface{})

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	addConfigToEnv("", m, ev)
	data, err := ProcessConfig(b, ev, escapeChar)

	if err != nil {
		fmt.Println(string(b))
		return err
	}

	if err := json.Unmarshal(data, conf); err != nil {
		return err
	}

	return nil
}

func addConfigToEnv(keyPrefix string, conf map[string]interface{}, ev *env.Env) {
	for k, v := range conf {
		key := keyPrefix + k
		switch t := v.(type) {
		case string:
			ev.Set(key, t)
		case int64:
			ev.Set(key, strconv.FormatInt(t, 10))
		case float64:
			ev.Set(key, strconv.FormatFloat(t, 'f', -1, 64))
		case map[string]interface{}:
			addConfigToEnv(key+"_", t, ev)
		}
	}
}

// ProcessConfig replaces references of environment varialbes for the given data
// Support variable syntax: $varname, ${varname}
func ProcessConfig(data []byte, e *env.Env, escapeChar rune) ([]byte, error) {
	var result []byte
	var sc scanner.Scanner
	sc.Init(bytes.NewReader(data))

DONE:
	for {
		switch ch := sc.Peek(); ch {
		default:
			result = append(result, byte(sc.Next()))
		case scanner.EOF:
			break DONE
		case escapeChar:
			curr, next := sc.Next(), sc.Peek()
			if next != '$' {
				result = append(result, byte(curr))
			}

			if next != scanner.EOF {
				result = append(result, byte(sc.Next()))
			}

		case '$':
			name, err := parseVariable(&sc)

			if err != nil {
				pos := sc.Pos()
				return result, fmt.Errorf(`parseError:%d:%d: %v %q`, pos.Line, pos.Offset, err, name)
			}
			result = append(result, e.Get(string(name))...)
		}
	}
	return result, nil
}

func parseVariable(sc *scanner.Scanner) (result []byte, err error) {
	delims := []byte{byte(sc.Next())}

	if ch := sc.Peek(); ch == '{' {
		delims = append(delims, byte(sc.Next()))
	}

	name, err := parseName(sc)

	if err == nil && len(delims) > 1 && '}' != byte(sc.Peek()) {
		err = errInvalidSyntax
	}

	if err != nil {
		name = append(delims, name...)
		if len(delims) > 1 {
			name = append(name, byte(sc.Next()))
		}

		return name, err
	}

	if len(delims) > 1 {
		sc.Next()
	}
	return name, err
}

func parseName(sc *scanner.Scanner) (result []byte, err error) {
	if ch := sc.Peek(); scanner.EOF == ch {
		return result, errInvalidSyntax
	}

	for {
		if ch := sc.Peek(); unicode.IsLetter(ch) || unicode.IsDigit(ch) || '_' == ch {
			result = append(result, byte(sc.Next()))
		} else {
			if len(result) == 0 {
				err = errInvalidSyntax
			}
			return result, err
		}
	}
}
