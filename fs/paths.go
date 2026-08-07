package fs

import (
	"net/url"
	"path"
	"strings"
)

func normalizeCwd(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme == "file" {
		value = parsed.Path
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}

func inputPath(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme == "file" {
		return parsed.Path
	}
	return value
}
