package path

import "strings"

type win32Impl struct {
	cwd func() string
}

func (win32Impl) sep() string       { return winSep }
func (win32Impl) delimiter() string { return winDelimiter }

func (win32Impl) toNamespacedPath(p string) string { return p }

func toBackslash(p string) string {
	return strings.ReplaceAll(p, "/", "\\")
}

func isWinSeparator(c byte) bool { return c == '\\' || c == '/' }

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// splitWinPrefix splits a win32 path into its prefix (drive or UNC share or root)
// and the remaining relative segments.
func splitWinPrefix(p string) (prefix, rest string) {
	if len(p) >= 2 && isDriveLetter(p[0]) && (p[1] == ':' || p[1] == '|') {
		prefix = p[:2]
		p = p[2:]
		if len(p) > 0 && isWinSeparator(p[0]) {
			prefix += "\\"
			p = p[1:]
		}
		return prefix, p
	}
	if len(p) >= 2 && isWinSeparator(p[0]) && isWinSeparator(p[1]) {
		idx := 2
		for idx < len(p) && !isWinSeparator(p[idx]) {
			idx++
		}
		for idx < len(p) && isWinSeparator(p[idx]) {
			idx++
		}
		for idx < len(p) && !isWinSeparator(p[idx]) {
			idx++
		}
		prefix = p[:idx]
		return prefix, p[idx:]
	}
	if len(p) >= 1 && isWinSeparator(p[0]) {
		return "\\", p[1:]
	}
	return "", p
}

func cleanWinSegments(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, seg := range parts {
		switch seg {
		case "", ".":
			// skip
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, seg)
		}
	}
	return out
}

func isBareDrive(prefix string) bool {
	return len(prefix) == 2 && isDriveLetter(prefix[0]) && prefix[1] == ':'
}

func (win32Impl) normalize(p string) string {
	p = toBackslash(p)
	prefix, rest := splitWinPrefix(p)
	parts := strings.Split(rest, "\\")
	cleaned := cleanWinSegments(parts)
	if len(cleaned) == 0 {
		if prefix == "" {
			return "."
		}
		return prefix
	}
	out := prefix
	for i, seg := range cleaned {
		if i > 0 {
			out += "\\"
		} else if out != "" && !strings.HasSuffix(out, "\\") && !isBareDrive(out) {
			out += "\\"
		}
		out += seg
	}
	return out
}

func (w win32Impl) join(parts []string) string {
	joined := strings.Join(parts, "\\")
	return w.normalize(joined)
}

func (w win32Impl) resolve(parts []string) string {
	var resolved string
	absolute := false
	for i := len(parts) - 1; i >= -1 && !absolute; i-- {
		part := w.cwd()
		if i >= 0 {
			part = parts[i]
		}
		if part == "" {
			continue
		}
		part = toBackslash(part)
		resolved = part + "\\" + resolved
		absolute = win32Impl{}.isAbsolute(part)
	}
	return w.normalize(resolved)
}

func (win32Impl) isAbsolute(p string) bool {
	p = toBackslash(p)
	if len(p) >= 3 && isDriveLetter(p[0]) && (p[1] == ':' || p[1] == '|') && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	if len(p) >= 1 && isWinSeparator(p[0]) {
		return true
	}
	return false
}

func (w win32Impl) relative(from, to string) string {
	from = w.normalize(toBackslash(from))
	to = w.normalize(toBackslash(to))
	if from == to {
		return ""
	}
	fromPrefix, fromRest := splitWinPrefix(from)
	toPrefix, toRest := splitWinPrefix(to)
	if fromPrefix != toPrefix {
		return to
	}
	fromParts := cleanWinSegments(strings.Split(fromRest, "\\"))
	toParts := cleanWinSegments(strings.Split(toRest, "\\"))
	i := 0
	for i < len(fromParts) && i < len(toParts) && fromParts[i] == toParts[i] {
		i++
	}
	rel := make([]string, 0, len(fromParts)-i+len(toParts)-i)
	for j := i; j < len(fromParts); j++ {
		rel = append(rel, "..")
	}
	rel = append(rel, toParts[i:]...)
	if len(rel) == 0 {
		return "."
	}
	return strings.Join(rel, "\\")
}

func (win32Impl) basename(p string) string {
	p = toBackslash(p)
	p = strings.TrimRight(p, "\\")
	if p == "" {
		return ""
	}
	idx := strings.LastIndexByte(p, '\\')
	if idx == -1 {
		return p
	}
	return p[idx+1:]
}

func (w win32Impl) basenameExt(p, ext string) string {
	base := w.basename(p)
	if ext != "" && strings.HasSuffix(base, ext) && base != ext {
		return base[:len(base)-len(ext)]
	}
	return base
}

func (w win32Impl) dirname(p string) string {
	p = toBackslash(p)
	prefix, rest := splitWinPrefix(p)
	if rest == "" {
		return p
	}
	rest = strings.TrimRight(rest, "\\")
	idx := strings.LastIndexByte(rest, '\\')
	if idx == -1 {
		return prefix
	}
	out := prefix + rest[:idx]
	if out == "" {
		return "."
	}
	return out
}

func (w win32Impl) extname(p string) string {
	base := w.basename(p)
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return ""
	}
	if dot == len(base)-1 {
		return ""
	}
	return base[dot:]
}

func (w win32Impl) parse(p string) (root, dir, base, name, ext string) {
	p = toBackslash(p)
	root, _ = splitWinPrefix(p)
	if root == "" {
		root = ""
	}
	base = w.basename(p)
	if base == "" {
		base = root
		if base == "\\" {
			base = "\\"
		}
	}
	ext = w.extname(p)
	if ext != "" {
		name = base[:len(base)-len(ext)]
	} else {
		name = base
	}
	dir = w.dirname(p)
	if dir == "." {
		dir = ""
	}
	if dir == root {
		dir = root
	}
	return
}

func (w win32Impl) format(root, dir, base string) string {
	if dir == "" {
		return toBackslash(root) + base
	}
	dir = toBackslash(dir)
	if base == "\\" && dir == root {
		return toBackslash(root)
	}
	if dir == root {
		return toBackslash(root) + base
	}
	if strings.HasSuffix(dir, "\\") {
		return dir + base
	}
	return dir + "\\" + base
}
