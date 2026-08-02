package runner

import _ "embed"

//go:embed entrypoint.sh
var entrypoint []byte

// EntrypointScript returns the container entrypoint, written into the run
// dir and mounted read-only at /source/entrypoint.sh.
func EntrypointScript() []byte { return entrypoint }
