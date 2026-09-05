package main

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// ytIDRe matches a canonical YouTube video id: exactly 11 chars from the
// [A-Za-z0-9_-] alphabet. This whitelist is also what makes the id safe to
// splice into filesystem paths (no path separators, no "..") and into a URL —
// it is the single gate every output path derives from.
var ytIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// isYouTubeHost reports whether host is youtube.com / youtu.be, or a subdomain
// of either (e.g. m.youtube.com, music.youtube.com). Exact-suffix matching
// rejects lookalikes like "youtube.com.evil.example" or "notyoutube.com".
func isYouTubeHost(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range []string{"youtube.com", "youtu.be"} {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// loadURLs reads video URLs from a plain-text file: one URL per line, blank
// lines and lines starting with '#' are ignored. Defaults live in a root txt
// file (e.g. YT-URL-TEST.txt) — nothing is ever fetched outside this project.
func loadURLs(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read url file: %w", err)
	}
	var out []string
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no URLs found in %s", path)
	}
	return out, nil
}

// videoID extracts the 11-char YouTube video id from common URL shapes:
//
//	https://youtu.be/<id>?si=...
//	https://www.youtube.com/watch?v=<id>&list=...
//	https://www.youtube.com/shorts/<id>
//	https://www.youtube.com/embed/<id>
//	https://www.youtube.com/live/<id>
func videoID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse URL %q: %w", raw, err)
	}
	host := strings.ToLower(u.Hostname())
	if !isYouTubeHost(host) {
		return "", fmt.Errorf("%q is not a youtube URL", raw)
	}
	var id string
	switch {
	case strings.HasSuffix(host, "youtu.be"):
		id = strings.Trim(u.Path, "/")
	case u.Path == "/watch" || strings.HasPrefix(u.Path, "/watch/"):
		id = u.Query().Get("v")
	case strings.HasPrefix(u.Path, "/shorts/"):
		id = strings.TrimPrefix(u.Path, "/shorts/")
	case strings.HasPrefix(u.Path, "/embed/"):
		id = strings.TrimPrefix(u.Path, "/embed/")
	case strings.HasPrefix(u.Path, "/live/"):
		id = strings.TrimPrefix(u.Path, "/live/")
	default:
		// Fall back to a bare id as the path (youtu.be handled above) or ?v=.
		id = u.Query().Get("v")
		if id == "" {
			id = strings.Trim(u.Path, "/")
		}
	}
	id = strings.Trim(id, "/")
	if !ytIDRe.MatchString(id) {
		return "", fmt.Errorf("could not find a valid 11-char video id in %q", raw)
	}
	return id, nil
}
