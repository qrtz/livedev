package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type ServerConf struct {
	Default bool     `json:"default"`
	Skip    string   `json:"skip"`
	Host    string   `json:"host"`
	Port    int      `json:"port"`
	Bin     string   `json:"bin"`
	Source  []string `json:"source"`
	Target  string   `json:"target"`
	Startup []string `json:"startup"`
}

type Config struct {
	Port   int          `json:"addr"` //proxy port
	GOROOT string       `json:"GOROOT"`
	GOPATH string       `json:"GOPATH"`
	Server []ServerConf `json:"server"`
}

func LoadConfig(configFile string) (*Config, error) {
	r, err := os.Open(configFile)

	if err != nil {
		return nil, fmt.Errorf("Unable to read configution file: %s\n%s", configFile, err.Error())
	}

	conf := new(Config)

	dec := json.NewDecoder(r)
	if err := dec.Decode(&conf); err != nil {
		return nil, fmt.Errorf("Unable to parse configution file: %s\n%s", configFile, err.Error())
	}

	return conf, nil
}
