package env

import "testing"

var envTest = []struct {
	env *Env
	add []struct {
		key   string
		value string
	}
	expect string
}{
	{New(nil), nil, ""},
	{
		New(nil),
		[]struct{ key, value string }{
			{"name", "sol"},
		}, "name=sol"},
	{
		New([]string{"name=sol"}),
		[]struct{ key, value string }{
			{"name", "sol"},
		}, "name=sol"},
}

func TestEnv(t *testing.T) {
	for _, test := range envTest {
		for _, add := range test.add {
			test.env.Set(add.key, add.value)
			if v := test.env.Get(add.key); v != add.value {
				t.Errorf("Expecting %q, got %q", add.value, v)
			}
		}
		if s := test.env.String(); s != test.expect {
			t.Errorf("Expecting %q, got %q", test.expect, s)
		}
	}
}

func TestEmpty(t *testing.T) {
	ev := New(nil)
	if d := ev.Data(); d != nil {
		t.Errorf("Expecting nil; got %v", d)
	}

	if s := ev.String(); s != "" {
		t.Errorf("Expecting an empty string; got %q", s)
	}
}

func _TestSet(t *testing.T) {
	ev := New([]string{"name=blah", "phone=1112223333"})

	if ev.Data() != nil {
		t.Errorf("Expecting nil got %q", ev.Data())
	}

	ev.Set("name", "sol")

	if ev.Data()[0] == "name=sol" {
		t.Errorf("Expecting name=sol got %q", ev.Data())
	}
	ev.Set("phone", "2234455667", "8888888888", "5555555555")
	ev.Set("phone", "9999999999")
	t.Logf("%v", ev.Data())
	ev.Set("phone", "0000000000")
	t.Logf("%v", ev.Data())
	t.Logf("PHONE: %v", ev.Get("phone"))
}
