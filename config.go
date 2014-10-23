package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type ServerConf struct {
	Default        bool          `json:"default"`
	Host           string        `json:"host"`
	Port           int           `json:"port"`
	Bin            string        `json:"bin"`
	Resources      ResourceConf  `json:"resources"`
	Target         string        `json:"target"`
	Startup        []string      `json:"startup"`
	Builder        []string      `json:"builder"`
	Workspace      string        `json:"workspace"`
	GOROOT         string        `json:"GOROOT"`
	GOPATH         []string      `json:"GOPATH"`
	StartupTimeout time.Duration `json:timeout`
}

type ResourceConf struct {
	Ignore string   `json:"ignore"`
	Paths  []string `json:"paths"`
}

type Config struct {
	Port   int          `json:"port"` //proxy port
	GOROOT string       `json:"GOROOT"`
	GOPATH []string     `json:"GOPATH"`
	Server []ServerConf `json:"server"`
}

func LoadConfig(configFile string) (*Config, error) {
	r, err := os.Open(configFile)

	if err != nil {
		return nil, fmt.Errorf("Unable to read configution file: %s\n%s", configFile, err.Error())
	}

	defer r.Close()

	conf := new(Config)

	dec := json.NewDecoder(r)
	if err := dec.Decode(&conf); err != nil {
		return nil, fmt.Errorf("Unable to parse configution file: %s\n%s", configFile, err.Error())
	}

	return conf, nil
}
