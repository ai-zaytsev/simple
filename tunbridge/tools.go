//go:build tools

// The binding generator is a build-time dependency of this module rather than
// something the code imports. Without it recorded here, gomobile refuses to
// run: it requires its own package to be in the module graph. The build tag
// keeps it out of the compiled library.
package tunbridge

import (
	_ "golang.org/x/mobile/bind"
)
