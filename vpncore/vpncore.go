// Package vpncore is the single native library the application links against.
//
// It deliberately binds two separate upstream projects into one package. Each
// gomobile binding carries its own copy of the Go runtime, so two independent
// AARs cannot coexist in one APK: the build fails on duplicated runtime
// classes. Binding both here produces one runtime, one artifact and one
// checksum to record.
//
// The two halves are:
//
//   - the transport engine (Xray through libXray), which speaks the protocol
//     to the node and exposes a local SOCKS proxy;
//   - the packet bridge (tun2socks), which moves packets between the device
//     TUN interface and that proxy.
//
// Neither half alone establishes a tunnel. The engine contains no TUN handling
// whatsoever, so without the bridge the application connects to the node while
// every packet still travels outside the tunnel.
//
// The exported surface is deliberately small and made of simple types, because
// gomobile only carries simple types across the language boundary.
package vpncore

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"
	"github.com/xjasonlyu/tun2socks/v2/dialer"
	t2slog "github.com/xjasonlyu/tun2socks/v2/log"
	"github.com/xjasonlyu/tun2socks/v2/proxy"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"github.com/xtls/libxray/xray"
	"gvisor.dev/gvisor/pkg/tcpip/stack"

	// Registers the socks5 scheme with the parser above. That parser is a
	// registry filled by package init functions, so a protocol nobody imports
	// simply does not exist and parsing fails with "proxy: unknown protocol".
	// Assembling the stack by hand means these registrations are ours to make:
	// the entry point we deliberately avoid is also what pulls them in
	// upstream.
	_ "github.com/xjasonlyu/tun2socks/v2/proxy/socks5"
)

// assetEnv is where the engine looks for its geographic database. The routing
// rules reference geoip entries, and the engine refuses a configuration whose
// referenced database it cannot find, so this must be set before it starts.
const assetEnv = "XRAY_LOCATION_ASSET"

// bridgeLogLevel keeps destinations out of the device log. The bridge logs
// every connection at info level, which is exactly the browsing history the
// privacy model forbids retaining, so the level is fixed here rather than left
// at the library default.
const bridgeLogLevel = "warning"

// Protector hands a socket to the platform so that it bypasses the tunnel.
// Implemented on the application side by the VPN service.
type Protector interface {
	Protect(fd int) bool
}

// SetProtector installs the protector used for every socket the engine opens.
//
// An unprotected socket that the engine opens towards the node is itself
// routed into the tunnel being established, which feeds the engine its own
// traffic: the loop that makes a VPN appear to hang. The application also
// excludes its own package from the tunnel, so this is the second of two
// independent guards against the same failure.
func SetProtector(p Protector) error {
	if p == nil {
		return errors.New("protector is nil")
	}
	registerProtector(func(fd uintptr) bool {
		return p.Protect(int(fd))
	})
	return nil
}

// SetAssetDir points the engine at the directory holding its geographic
// database. The directory must exist and already contain the database.
func SetAssetDir(dir string) error {
	if dir == "" {
		return errors.New("asset directory is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}
	return os.Setenv(assetEnv, dir)
}

// StartEngine runs the transport engine with the given Xray configuration.
func StartEngine(configJSON string) (err error) {
	// A panic crossing into Java takes the process down and leaves the system
	// VPN slot in an unclear state. A refused configuration must surface as a
	// reported failure instead.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine panicked while starting: %v", r)
		}
	}()

	if configJSON == "" {
		return errors.New("engine configuration is empty")
	}
	return xray.RunXray(configJSON)
}

// StopEngine stops the engine. Safe to call when it is not running.
func StopEngine() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine panicked while stopping: %v", r)
		}
	}()
	return xray.StopXray()
}

// EngineRunning asks the engine itself rather than a cached flag.
func EngineRunning() bool {
	return xray.GetXrayState()
}

// EngineVersion reports the bundled Xray version, for matching a device
// against a build when a report comes in.
func EngineVersion() string {
	return xray.XrayVersion()
}

var (
	bridgeMu     sync.Mutex
	bridgeDevice device.Device
	bridgeStack  *stack.Stack
)

// BridgeStats reports what the bridge has actually seen, as one short line
// meant to be read off a screen and repeated over a chat.
//
// It exists because "connected but nothing loads" has several causes that look
// identical from the outside, and they are told apart by which number is zero.
// No packets received means the interface is not feeding the bridge at all.
// Packets received with no connections opened means they arrive and go
// nowhere. Connections opened with nothing sent means the far side is the
// problem. Without this, each guess costs an install.
func BridgeStats() string {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	if bridgeStack == nil {
		return "bridge stopped"
	}

	s := bridgeStack.Stats()
	return fmt.Sprintf(
		"ip in=%d out=%d bad=%d | tcp new=%d live=%d | udp in=%d out=%d",
		s.IP.PacketsReceived.Value(),
		s.IP.PacketsSent.Value(),
		s.IP.MalformedPacketsReceived.Value(),
		s.TCP.ActiveConnectionOpenings.Value(),
		s.TCP.CurrentEstablished.Value(),
		s.UDP.PacketsReceived.Value(),
		s.UDP.PacketsSent.Value(),
	)
}

// StartBridge attaches a TUN file descriptor to the engine's local proxy.
//
// Ownership of fd moves here: the caller must not close it after a successful
// call, and StopBridge releases it. That is why the caller detaches the
// descriptor from its Java owner rather than passing a borrowed one, which
// would end up closed twice.
//
// mtu must match the value the interface was built with, and proxyURL is a
// full URL such as socks5://127.0.0.1:10808.
//
// This assembles the stack directly instead of using the bridge library's own
// engine entry point, which reports a bad key by calling the equivalent of
// exit: on a phone that is not a failed connection, it is a vanished
// application.
func StartBridge(fd int, mtu int, proxyURL string) (err error) {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	if bridgeDevice != nil || bridgeStack != nil {
		return errors.New("bridge is already running")
	}
	if fd <= 0 {
		return fmt.Errorf("invalid file descriptor: %d", fd)
	}
	if mtu <= 0 {
		return fmt.Errorf("invalid mtu: %d", mtu)
	}
	if proxyURL == "" {
		return errors.New("proxy address is empty")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bridge panicked while starting: %v", r)
		}
		// A half-built stack holds the descriptor and forwards nothing. Undo
		// it here so that a retry starts from a clean state.
		if err != nil {
			closeBridgeLocked()
		}
	}()

	level, err := t2slog.ParseLevel(bridgeLogLevel)
	if err != nil {
		return err
	}
	logger, err := t2slog.NewLeveled(level)
	if err != nil {
		return err
	}
	t2slog.SetLogger(logger)

	// The dialer keeps options from a previous session. Reset before the new
	// one so that a reconnect does not inherit the old network's settings.
	dialer.Reset()

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}
	forwarder, err := proxy.Parse(parsed)
	if err != nil {
		return err
	}
	tunnel.T().SetProxy(forwarder)

	dev, err := fdbased.Open(strconv.Itoa(fd), uint32(mtu), 0)
	if err != nil {
		return err
	}
	bridgeDevice = dev

	created, err := core.CreateStack(&core.Config{
		LinkEndpoint:     dev,
		TransportHandler: tunnel.T(),
	})
	if err != nil {
		return err
	}
	bridgeStack = created

	return nil
}

// StopBridge detaches the interface and releases the descriptor. Safe to call
// when the bridge is not running.
func StopBridge() {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()

	// Teardown runs on paths that are already handling another failure, so a
	// failure here must not replace the original one.
	defer func() {
		_ = recover()
	}()

	closeBridgeLocked()
}

// BridgeRunning reports whether packets are currently being forwarded.
func BridgeRunning() bool {
	bridgeMu.Lock()
	defer bridgeMu.Unlock()
	return bridgeDevice != nil && bridgeStack != nil
}

func closeBridgeLocked() {
	if bridgeDevice != nil {
		bridgeDevice.Close()
		bridgeDevice = nil
	}
	if bridgeStack != nil {
		bridgeStack.Close()
		bridgeStack.Wait()
		bridgeStack = nil
	}
}
