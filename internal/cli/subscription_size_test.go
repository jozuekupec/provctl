package cli

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		valid bool
	}{
		{"", 0, true}, {"20G", 20 * 1024 * 1024 * 1024, true}, {"2m", 2 * 1024 * 1024, true}, {"1T", 1024 * 1024 * 1024 * 1024, true}, {"0", 0, false}, {"20GB", 0, false}, {"-1G", 0, false},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := parseByteSize(test.input)
			if test.valid && err != nil {
				t.Fatalf("parseByteSize(%q) error = %v", test.input, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("parseByteSize(%q) succeeded", test.input)
			}
			if got != test.want {
				t.Errorf("parseByteSize(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}
