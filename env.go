package main

import (
	"os"
	"sync"
)

const pathListSeparator = string(os.PathListSeparator)

type Env struct {
	keys map[string]int
	data []string
	lock sync.Mutex
}

func NewEnv(data []string) *Env {
	env := &Env{
		keys: make(map[string]int),
		data: data,
	}
	return env.fillkeys()
}

func (env *Env) fillkeys() *Env {
	for i, v := range env.data {
		for j, k := 0, len(v); j < k; j++ {
			if v[j] == '=' {
				key := v[:j]
				if _, ok := env.keys[key]; !ok {
					env.keys[key] = i
				}
				break
			}
		}
	}

	return env
}

func (env *Env) Add(key, val string) {
	env.lock.Lock()
	defer env.lock.Unlock()
	i, ok := env.keys[key]
	if ok {
		env.data[i] += pathListSeparator + val
		return
	}
	env.keys[key] = len(env.data)
	env.data = append(env.data, key+"="+val)
}

func (env *Env) Set(key, val string) {
	env.lock.Lock()
	defer env.lock.Unlock()

	v := key + "=" + val
	if i, ok := env.keys[key]; ok {
		env.data[i] = v
		return
	}

	env.keys[key] = len(env.data)
	env.data = append(env.data, v)
}

func (env *Env) Data() []string {
	return env.data
}

