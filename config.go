package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type serverConfig struct {
	Default        bool           `json:"default"`
	Host           string         `json:"host"`
	Port           int            `json:"port"`
	Bin            string         `json:"bin"`
	Resources      resourceConfig `json:"resources"`
	Target         string         `json:"target"`
	Startup        []string       `json:"startup"`
	Builder        []string       `json:"builder"`
	Workspace      string         `json:"workspace"`
	GoRoot         string         `json:"GOROOT"`
	GoPath         []string       `json:"GOPATH"`
	StartupTimeout time.Duration  `json:"startupTimeout"`
}

type resourceConfig struct {
	Ignore string   `json:"ignore"`
	Paths  []string `json:"paths"`
}

type config struct {
	Port    int            `json:"port"` //proxy port
	GoRoot  string         `json:"GOROOT"`
	GoPath  []string       `json:"GOPATH"`
	Servers []serverConfig `json:"server"`
}

func loadConfig(configFile string) (*config, error) {
	r, err := os.Open(configFile)

	if err != nil {
		return nil, fmt.Errorf("Unable to read configution file: %s\n%s", configFile, err.Error())
	}

	defer r.Close()

	conf := new(config)

	dec := json.NewDecoder(r)
	if err := dec.Decode(&conf); err != nil {
		return nil, fmt.Errorf("Unable to parse configution file: %s\n%s", configFile, err.Error())
	}

	return conf, nil
}
