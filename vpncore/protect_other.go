//go:build !android

package vpncore

// registerProtector does nothing off Android, where there is no tunnel to
// escape from. The stub exists so that the package compiles and can be vetted
// on the build machine, which is not an Android host.
func registerProtector(protect func(uintptr) bool) {
	_ = protect
}
