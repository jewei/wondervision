//go:build e2e

package main

// Test builds only. Production builds contain no automation socket.
import _ "github.com/egoist/mygo/plugins/automation"
