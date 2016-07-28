package main

import (
	"github.com/qrtz/livedev/env"
	"testing"
)

const escapeChar = '`'

var data = []struct {
	input  string
	expect string
	pass   bool
}{
	{"`$", `$`, true},
	{"`", "`", true},
	{"", "", true},
	{"Hello,$NAME", "Hello,world", true},
	{"\\", "\\", true},
	{"$KIND${NAME}", "braveworld", true},
	{"${UNDEFINED}", "", true},

	{"$", "", false},
	{"${}", "", false},
	{"${NAME", "", false},
	{"$ NAME ", "", false},
}

var ev = env.New([]string{
	"NAME=world",
	"KIND=brave",
})

func TestProcessConfig(t *testing.T) {

	for _, test := range data {
		result, err := ProcessConfig([]byte(test.input), ev, escapeChar)
		if err != nil {
			t.Log(err)
			if test.pass {
				t.Fatal(err, string(result), test.input)
			}
		} else if test.pass == false {
			t.Fatal("Expected failure", string(result), test.input)

		} else if test.expect != string(result) {
			t.Fatalf("Expected: %q got %q", test.expect, string(result))
		}

		t.Logf("Expected: %q got %q", test.expect, string(result))
	}
}

func BenchmarkProcessConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		data := []byte(`Hello $KIND ${NAME} and $UNDEFINED`)
		result, err := ProcessConfig(data, ev, escapeChar)
		if err != nil {
			b.Fatal(err, string(result))
		}
	}
}
