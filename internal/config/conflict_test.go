package config

import "testing"

func TestCheckConflict(t *testing.T) {
	cases := []struct {
		name  string
		scope map[string]string
		key   string
		value string
		ok    bool
	}{
		{
			name:  "forward: setting git.token while git.type=bot",
			scope: map[string]string{"git.type": "bot"},
			key:   "git.token",
			value: "tok",
			ok:    false,
		},
		{
			name:  "forward: setting git.app.id while git.type=user",
			scope: map[string]string{"git.type": "user"},
			key:   "git.app.id",
			value: "123",
			ok:    false,
		},
		{
			name:  "reverse: git.token already set, setting git.type=bot",
			scope: map[string]string{"git.token": "tok"},
			key:   "git.type",
			value: "bot",
			ok:    false,
		},
		{
			name:  "reverse: git.app.id already set, setting git.type=user",
			scope: map[string]string{"git.app.id": "123"},
			key:   "git.type",
			value: "user",
			ok:    false,
		},
		{
			name:  "no conflict: git.type unset constrains nothing",
			scope: map[string]string{},
			key:   "git.token",
			value: "tok",
			ok:    true,
		},
		{
			name:  "no conflict: git.type bot with app.* fields",
			scope: map[string]string{"git.type": "bot"},
			key:   "git.app.id",
			value: "123",
			ok:    true,
		},
		{
			name:  "no conflict: git.type user with token",
			scope: map[string]string{"git.type": "user"},
			key:   "git.token",
			value: "tok",
			ok:    true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckConflict(c.scope, c.key, c.value)
			if (err == nil) != c.ok {
				t.Errorf("CheckConflict(%v, %q, %q) = %v, want ok=%v", c.scope, c.key, c.value, err, c.ok)
			}
		})
	}
}
