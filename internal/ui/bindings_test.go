package ui

import (
	"strings"
	"testing"
)

func TestBindings_EveryAdvertisedKeyResolvesToItsAction(t *testing.T) {
	for _, binding := range bindings {
		for _, context := range binding.Contexts {
			for _, key := range binding.Keys {
				if got := actionFor(context, key); got != binding.Action {
					t.Errorf("context %d key %q = %q, want %q", context, key, got, binding.Action)
				}
			}
		}
	}
}

func TestBindings_KeybarAndHelpShareSource(t *testing.T) {
	for _, context := range append(append([]shortcutContext{}, contextsAll...), contextsEditor...) {
		help := helpFor(context)
		if len(help) == 0 {
			t.Errorf("context %d has no help", context)
		}
		for _, binding := range bindingsFor(context) {
			if binding.Bar == "" {
				continue
			}
			if got := keybarFor(context); !containsString(got, binding.Bar) {
				t.Errorf("context %d keybar %q omits %q", context, got, binding.Bar)
			}
		}
	}
}

func containsString(value, fragment string) bool {
	return strings.Contains(value, fragment)
}
