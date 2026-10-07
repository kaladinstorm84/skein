package repo

import "testing"

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"hello":           "hello",
		"Hello From Demo": "hello-from-demo",
		"  Fix: API  ":    "fix-api",
		"!!!":             "",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Fatalf("Slug(%q)=%q want %q", in, got, want)
		}
	}
}

func TestShortID(t *testing.T) {
	id := "81ba15cc9bfc0123456789abcdef0123456789abcdef0123456789abcdef0123"
	if got := ShortID(id); got != "81ba15cc9bfc" {
		t.Fatalf("ShortID=%q", got)
	}
	if ShortID("abc") != "abc" {
		t.Fatal("short ids should pass through")
	}
}
