package server

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input      string
		wantAction string
		wantPlugin string
	}{
		{"reload-jira", "reload", "jira"},
		{"reload-all", "reload", "all"},
		{"reload-google-calendar", "reload", "google-calendar"},
		{"list", "list", ""},
		{"reload", "reload", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, plugin := parseCommand(tt.input)
			if action != tt.wantAction {
				t.Errorf("action = %q, want %q", action, tt.wantAction)
			}
			if plugin != tt.wantPlugin {
				t.Errorf("plugin = %q, want %q", plugin, tt.wantPlugin)
			}
		})
	}
}
