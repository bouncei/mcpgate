package policy

import "testing"

func TestAllowed(t *testing.T) {
	cases := []struct {
		name  string
		allow []string
		tool  string
		want  bool
	}{
		{"explicit allow", []string{"read_file"}, "read_file", true},
		{"not in list", []string{"read_file"}, "run_command", false},
		{"wildcard", []string{"*"}, "anything", true},
		{"empty deny-by-default", []string{}, "read_file", false},
		{"nil deny-by-default", nil, "read_file", false},
		{"multiple", []string{"a", "b", "read_file"}, "read_file", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Allowed(c.allow, c.tool); got != c.want {
				t.Errorf("Allowed(%v, %q) = %v, want %v", c.allow, c.tool, got, c.want)
			}
		})
	}
}
