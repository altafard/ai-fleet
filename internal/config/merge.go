package config

import "path/filepath"

// Merge overlays local onto global (local wins per key). The private-key
// path is resolved against the scope that supplied it — a relative path in
// a config file must mean the same thing from any working directory.
func Merge(local, global map[string]string, root, home string) map[string]string {
	out := map[string]string{}
	for k, v := range global {
		out[k] = resolveKeyPath(k, v, home)
	}
	for k, v := range local {
		out[k] = resolveKeyPath(k, v, root)
	}
	return out
}

func resolveKeyPath(key, value, base string) string {
	if key != "git.app.private-key" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(base, value)
}
