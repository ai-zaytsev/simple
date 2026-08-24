//go:build android

package vpncore

import "github.com/xtls/libxray/controller"

// registerProtector wires the platform protector into the engine's socket
// creation. Both directions are registered: an unprotected inbound listener
// socket loops exactly as an outbound one does.
func registerProtector(protect func(uintptr) bool) {
	controller.RegisterDialerController(protect)
	controller.RegisterListenerController(protect)
}
