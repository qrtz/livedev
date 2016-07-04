package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type serverConfig struct {
	Default        bool              `json:"default"`
	Host           string            `json:"host"`
	Port           int               `json:"port"`
	Bin            string            `json:"bin"`
	Resources      resourceConfig    `json:"resources"`
	Assets         resourceConfig    `json:"assets"`
	Target         string            `json:"target"`
	Startup        []string          `json:"startup"`
	Builder        []string          `json:"builder"`
	GoRoot         string            `json:"GOROOT,omitempty"`
	GoPath         []string          `json:"GOPATH,omitempty"`
	StartupTimeout time.Duration     `json:"startupTimeout,omitempty"`
	Env            map[string]string `json:"env"`
}

type resourceConfig struct {
	Ignore string   `json:"ignore"`
	Paths  []string `json:"paths"`
}

type config struct {
	Port    int            `json:"port,omitempty"` //proxy port
	GoRoot  string         `json:"GOROOT,omitempty"`
	GoPath  []string       `json:"GOPATH"`
	Servers []serverConfig `json:"server"`
}

func (c *config) UnmarshalJSON(data []byte) error {
	type cfg config
	var conf cfg
	err := json.Unmarshal(data, &conf)
	if err != nil {
		return err
	}

	if conf.Port > 0 {
		c.Port = conf.Port
	}

	if len(conf.GoRoot) == 0 {
		c.GoRoot = conf.GoRoot
	}

	conf.GoPath = append(c.GoPath, conf.GoPath...)

	for i := range conf.Servers {
		s := &conf.Servers[i]
		if len(s.GoPath) == 0 {
			s.GoPath = conf.GoPath
		}

		if len(s.GoRoot) == 0 {
			s.GoRoot = conf.GoRoot
		}

		if len(s.Host) == 0 {
			s.Host = "localhost"
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
