//go:build tools

package vpncore

// The binding generator is a build-time tool rather than a runtime import, but
// it refuses to run unless it appears in the module graph. This import is how
// it gets there, and the tools tag keeps it out of the shipped library.
import _ "golang.org/x/mobile/bind"
