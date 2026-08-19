// Package version holds the binary version and the client identity we present
// to upstream APIs.
//
// Why this exists as its own package: both the LLM clients and the tools need
// the User-Agent, and neither should import the other just for a string.
//
// On identifying honestly: providers of subscription plans (Kimi Code, Z.ai
// GLM Coding Plan) explicitly forbid spoofing the client identity — Kimi's
// community guidelines say "Don't spoof or alter client identity information".
// We therefore never pretend to be Claude Code or any other client, and we do
// not hide behind Go's default "Go-http-client/2.0" either: every request says
// who we are and links to the source.
package version

import "sync/atomic"

const (
	// Repo is where the client can be inspected — part of the User-Agent so a
	// provider seeing unfamiliar traffic can find out what it is.
	Repo = "https://github.com/execai/execai-agent"
	// Product is the client identifier. Keep it stable: providers whitelist
	// clients by this name.
	Product = "execai-agent"
)

var current atomic.Value // string

func init() { current.Store("dev") }

// Set is called once from main with the value baked in at build time.
func Set(v string) {
	if v != "" {
		current.Store(v)
	}
}

// Get returns the binary version ("dev" for local builds).
func Get() string {
	if v, ok := current.Load().(string); ok {
		return v
	}
	return "dev"
}

// UserAgent is the identity sent to every upstream HTTP API.
func UserAgent() string {
	return Product + "/" + Get() + " (+" + Repo + ")"
}
