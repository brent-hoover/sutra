// Package config loads and validates configuration from a TOML file at
// $XDG_CONFIG_HOME/sutra/config.toml (default ~/.config/sutra/config.toml):
// the daemon listen address and DB path, the client host, the bearer token,
// and the Claude projects directory. A missing file uses built-in defaults.
package config
