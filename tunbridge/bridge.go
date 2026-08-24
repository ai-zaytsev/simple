// Package tunbridge moves packets between the Android TUN interface and the
// local proxy that the transport engine exposes.
//
// The engine speaks SOCKS on loopback and knows nothing about the device's
// network interface: it was checked against the engine sources, which contain
// no TUN handling at all. Without this package the application establishes a
// connection to the node and no device traffic ever reaches it.
//
// The whole surface is three functions, because gomobile only carries simple
// types across the boundary and a narrow surface is easier to keep correct
// than a faithful one.
package tunbridge

import (
	"errors"
	"fmt"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/engine"
)

var (
	mu      sync.Mutex
	running bool
)

// Start attaches the given TUN file descriptor to the proxy.
//
// Ownership of fd moves to this package: the caller must not close it after a
// successful call, and Stop releases it. That is why the caller detaches the
// descriptor from its Java owner rather than passing a borrowed one, which
// would be closed twice.
//
// proxy is a full URL, for example socks5://127.0.0.1:10808.
func Start(fd int, mtu int, proxy string) error {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return errors.New("bridge is already running")
	}
	if fd <= 0 {
		return fmt.Errorf("invalid file descriptor: %d", fd)
	}
	if proxy == "" {
		return errors.New("proxy address is empty")
	}

	key := &engine.Key{
		Device:   fmt.Sprintf("fd://%d", fd),
		Proxy:    proxy,
		MTU:      mtu,
		LogLevel: "warning",
	}

	engine.Insert(key)

	// Start panics rather than returning an error on a bad key, so it is
	// contained here: a panic crossing into Java would take the process down
	// and leave the system VPN slot in an unclear state.
	var startErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				startErr = fmt.Errorf("bridge failed to start: %v", r)
			}
		}()
		engine.Start()
	}()

	if startErr != nil {
		return startErr
	}

	running = true
	return nil
}

// Stop detaches the interface and releases the descriptor. Safe to call when
// the bridge is not running.
func Stop() {
	mu.Lock()
	defer mu.Unlock()

	if !running {
		return
	}

	func() {
		defer func() {
			// A failure while stopping must not propagate: teardown runs on
			// paths that are already handling another failure.
			_ = recover()
		}()
		engine.Stop()
	}()

	running = false
}

// IsRunning reports whether packets are currently being forwarded.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}
