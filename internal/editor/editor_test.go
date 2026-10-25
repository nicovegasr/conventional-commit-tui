package editor

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// stubEnv replaces the environment and the PATH lookup for one test.
func stubEnv(t *testing.T, env map[string]string, installed ...string) {
	t.Helper()
	origEnv, origLook := getenv, lookPath
	getenv = func(k string) string { return env[k] }
	lookPath = func(name string) (string, error) {
		if slices.Contains(installed, name) {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { getenv, lookPath = origEnv, origLook })
}

func TestArgvPrefersVisualThenEditor(t *testing.T) {
	stubEnv(t, map[string]string{"VISUAL": "nvim", "EDITOR": "nano"}, "micro")
	if got := Argv("a.go", 7); !reflect.DeepEqual(got, []string{"nvim", "+7", "a.go"}) {
		t.Errorf("Argv = %v", got)
	}
	stubEnv(t, map[string]string{"EDITOR": "code --wait"})
	if got := Argv("a.go", 7); !reflect.DeepEqual(got, []string{"code", "--wait", "-g", "a.go:7"}) {
		t.Errorf("Argv = %v", got)
	}
}

func TestArgvFallsBackToTheFirstInstalled(t *testing.T) {
	stubEnv(t, nil, "nano", "vi")
	if got := Argv("a.go", 3); !reflect.DeepEqual(got, []string{"nano", "+3", "a.go"}) {
		t.Errorf("Argv = %v", got)
	}
	stubEnv(t, nil)
	if got := Argv("a.go", 3); got[0] != "vi" {
		t.Errorf("with nothing installed Argv = %v, want vi", got)
	}
}

func TestArgvUsesPathColonLineWhereThePlusFormIsUnknown(t *testing.T) {
	for _, name := range []string{"hx", "/opt/bin/helix", "subl", "zed"} {
		stubEnv(t, map[string]string{"EDITOR": name})
		if got := Argv("a.go", 9); !reflect.DeepEqual(got, []string{name, "a.go:9"}) {
			t.Errorf("%s: Argv = %v", name, got)
		}
	}
}

func TestCommandRunsTheArgv(t *testing.T) {
	stubEnv(t, map[string]string{"EDITOR": "micro"})
	cmd := Command("a.go", 2)
	if want := []string{"micro", "+2", "a.go"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}
