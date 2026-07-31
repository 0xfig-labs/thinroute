package main

import "testing"

func TestIsManagementCommand(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{[]string{"serve"}, false},
		{[]string{"--version"}, false},
		{[]string{"providers", "status"}, true},
		{[]string{"usage"}, true},
		{[]string{"models", "list"}, true},
		{[]string{"config", "validate"}, true},
	} {
		if got := isManagementCommand(test.args); got != test.want {
			t.Fatalf("isManagementCommand(%v) = %v, want %v", test.args, got, test.want)
		}
	}
}
