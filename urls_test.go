package main

import "testing"

func TestVideoID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"youtu.be short", "https://youtu.be/mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"youtu.be with si param", "https://youtu.be/mtWN6oPIi1Y?si=r9KTbQrCvg8cPNCL", "mtWN6oPIi1Y"},
		{"watch with v", "https://www.youtube.com/watch?v=mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"watch with list and si", "https://www.youtube.com/watch?v=mtWN6oPIi1Y&list=PLx&si=abc", "mtWN6oPIi1Y"},
		{"shorts", "https://www.youtube.com/shorts/mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"embed", "https://www.youtube.com/embed/mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"live", "https://www.youtube.com/live/mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"m.youtube", "https://m.youtube.com/watch?v=mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"music.youtube", "https://music.youtube.com/watch?v=mtWN6oPIi1Y", "mtWN6oPIi1Y"},
		{"trailing slash", "https://youtu.be/mtWN6oPIi1Y/", "mtWN6oPIi1Y"},
		{"id with dash and underscore", "https://youtu.be/abC-_dEfG_h", "abC-_dEfG_h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := videoID(c.in)
			if err != nil {
				t.Fatalf("videoID(%q) error: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("videoID(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestVideoIDRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"not youtube", "https://example.com/watch?v=mtWN6oPIi1Y"},
		{"no id", "https://youtu.be/"},
		{"too short", "https://youtu.be/abc"},
		{"empty", ""},
		{"not a url", "mtWN6oPIi1Y"},
		{"lookalike host suffix", "https://youtube.com.evil.example/watch?v=mtWN6oPIi1Y"},
		{"lookalike host prefix", "https://notyoutube.com/watch?v=mtWN6oPIi1Y"},
		{"path traversal dots", "https://youtu.be/../../../.."},
		{"path traversal backslash", `https://youtu.be/..\..\..\..`},
		{"slash in id", "https://youtu.be/mtWN6oPI/i1Y"},
		{"space in id", "https://youtu.be/mtWN6oPI 1Y"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := videoID(c.in); err == nil {
				t.Fatalf("videoID(%q) expected error, got none", c.in)
			}
		})
	}
}
