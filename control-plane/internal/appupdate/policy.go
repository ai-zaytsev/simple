// Package appupdate owns the channel-neutral application version decision.
package appupdate

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const DirectAPK = "direct_apk"

// Policy is the one version decision every distribution channel obeys.
// Channel contains only the material needed to carry out that decision.
type Policy struct {
	LatestVersionCode       int                 `json:"latest_version_code"`
	LatestVersionName       string              `json:"latest_version_name"`
	MinSupportedVersionCode int                 `json:"min_supported_version_code"`
	Channels                map[string]Artifact `json:"channels"`
}

// Artifact is deliberately small. A direct APK needs both fields; a future
// Google Play adapter can ignore them and let Play resolve the package.
type Artifact struct {
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Status string

const (
	Current  Status = "current"
	Optional Status = "optional"
	Required Status = "required"
)

// Verdict derives mandatory state from the minimum. There is no second flag
// that could say "optional" while the minimum says the build is unsupported.
func (p Policy) Verdict(currentVersionCode int) Status {
	switch {
	case currentVersionCode < p.MinSupportedVersionCode:
		return Required
	case currentVersionCode < p.LatestVersionCode:
		return Optional
	default:
		return Current
	}
}

func (p Policy) Validate() error {
	if p.LatestVersionCode < 1 {
		return errors.New("latest version must be positive")
	}
	if strings.TrimSpace(p.LatestVersionName) == "" {
		return errors.New("latest version name is empty")
	}
	if p.MinSupportedVersionCode < 1 {
		return errors.New("minimum version must be positive")
	}
	if p.MinSupportedVersionCode > p.LatestVersionCode {
		return errors.New("minimum version exceeds latest")
	}
	if artifact, ok := p.Channels[DirectAPK]; ok {
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("direct APK is invalid: %w", err)
		}
	}
	return nil
}

func (a Artifact) Validate() error {
	parsed, err := url.Parse(a.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return errors.New("URL must be an absolute HTTPS address")
	}
	if !sha256Pattern.MatchString(a.SHA256) {
		return errors.New("SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
