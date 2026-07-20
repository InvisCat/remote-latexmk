package serverurl

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "bare host", input: "conoha", want: "http://conoha:8080"},
		{name: "bare host and port", input: "conoha:18080", want: "http://conoha:18080"},
		{name: "HTTP default", input: "http://conoha", want: "http://conoha:8080"},
		{name: "HTTPS standard default", input: "https://conoha", want: "https://conoha"},
		{name: "HTTPS explicit port", input: "https://conoha:8443/", want: "https://conoha:8443"},
		{name: "IPv4", input: "100.64.0.1", want: "http://100.64.0.1:8080"},
		{name: "bare IPv6", input: "2001:db8::1", want: "http://[2001:db8::1]:8080"},
		{name: "bare IPv4-mapped IPv6", input: "::ffff:192.0.2.1", want: "http://[::ffff:192.0.2.1]:8080"},
		{name: "IPv6", input: "http://[2001:db8::1]", want: "http://[2001:db8::1]:8080"},
		{name: "IPv6 and port", input: "[2001:db8::1]:18080", want: "http://[2001:db8::1]:18080"},
		{name: "base path", input: "paper.example/api/", want: "http://paper.example:8080/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeRejectsUnsafeOrAmbiguousAddresses(t *testing.T) {
	for _, input := range []string{
		"",
		"ftp://latex.example",
		"http://user:password@latex.example",
		"http://latex.example?token=value",
		"http://latex.example#fragment",
		"http://latex.example:",
		"http://latex.example:0",
		"http://latex.example:65536",
		"http://latex.example:not-a-port",
		"2001:db8::1:invalid",
		"http://latex.example\n.invalid",
	} {
		t.Run(input, func(t *testing.T) {
			if got, err := Normalize(input); err == nil {
				t.Fatalf("Normalize(%q) = %q, want error", input, got)
			}
		})
	}
}
