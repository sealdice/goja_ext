// Package extra is reserved for opt-in filesystem capabilities that are not
// part of the Afero-backed Deno FS core.
//
// TODO: install these APIs only through explicit extension registration:
//   - lstat/readlink/symlink via afero.Lstater, afero.LinkReader, and
//     afero.Linker when the backend provides them.
//   - hard links via a host capability, because Afero core has no hard-link
//     interface.
//   - watch, lock, terminal, raw mode, and realpath through dedicated host
//     capabilities.
//
// The fs package must not import this package. Keeping it separate lets
// embedders delete this directory without changing the core path, handle, or
// WHATWG Stream implementation.
package extra
