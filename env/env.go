package env

import (
	"os"
	"strings"
	"sync"
)

const pathListSeparator = string(os.PathListSeparator)

// Env represents an environment.
type Env struct {
	keys map[string]int
	data []string
	lock sync.Mutex
}

// New returns a new environment filled with initial data.
func New(data []string) *Env {
	env := &Env{
		keys: make(map[string]int),
	}

	if len(data) > 0 {
		env.data = make([]string, len(data))
		copy(env.data, data)
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

// Set sets the value of the environment variable named by the key.
func (env *Env) Set(key string, values ...string) {
	env.lock.Lock()
	defer env.lock.Unlock()

	value := key + "=" + strings.Join(values, pathListSeparator)

	if i, ok := env.keys[key]; ok {
		env.data[i] = value
	} else {
		env.keys[key] = len(env.data)
		env.data = append(env.data, value)
	}
}

// Find retrieves the value of the environment variable named by the given key. The  second return value
// indicates whether the key exists or not
func (env *Env) Find(key string) (value string, found bool) {
	i, found := env.keys[key]
	if found {
		data := env.data[i]
		value = data[strings.Index(data, "=")+1:]
	}

	return value, found
}

// Get retrieves the value of the environment variable named by the given key. The return value will be
// an empty string if the key is not present
func (env *Env) Get(key string) (value string) {
	value, _ = env.Find(key)
	return value
}

// Data returns a copy of the slice representing the environment.
func (env *Env) Data() []string {
	return env.data
}

// String return the string representation of Env data
func (env *Env) String() string {
	return strings.Join(env.Data(), "\n")
}
