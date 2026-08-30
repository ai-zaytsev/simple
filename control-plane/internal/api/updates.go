package api

import (
	"errors"
	"net/http"
	"strings"

	"download.simplevpn/control-plane/internal/appupdate"
	"download.simplevpn/control-plane/internal/store"
)

func (s *Server) adminUpdates(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	state, err := s.store.LoadServiceState(r.Context())
	if err != nil {
		s.log.Error("cannot read application update policy", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot read application update policy")
		return
	}
	writeJSON(w, http.StatusOK, state.AppUpdates)
}

// adminPublishUpdate is called only after the official publisher has fetched
// the public APK back, matched its hash and verified its signature.
func (s *Server) adminPublishUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var body struct {
		VersionCode int    `json:"version_code"`
		VersionName string `json:"version_name"`
		Channel     string `json:"channel"`
		URL         string `json:"url"`
		SHA256      string `json:"sha256"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	policy, err := s.store.PublishAppRelease(r.Context(), store.AppRelease{
		VersionCode: body.VersionCode,
		VersionName: body.VersionName,
		Channel:     body.Channel,
		Artifact: appupdate.Artifact{
			URL:    body.URL,
			SHA256: body.SHA256,
		},
	}, changedBy(r))
	if err != nil {
		if errors.Is(err, store.ErrUpdateRollback) ||
			errors.Is(err, store.ErrMinimumAboveLatest) ||
			errors.Is(err, store.ErrReleaseDisagreement) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.log.Info("application latest version changed",
		"version_code", policy.LatestVersionCode,
		"version_name", policy.LatestVersionName,
		"channel", body.Channel,
		"by", changedBy(r))
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) adminMinimumUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	var body struct {
		VersionCode *int `json:"version_code"`
	}
	if err := decode(w, r, &body); err != nil || body.VersionCode == nil {
		writeError(w, http.StatusBadRequest, "version_code is required")
		return
	}

	policy, err := s.store.SetMinSupportedAppVersion(
		r.Context(), *body.VersionCode, changedBy(r),
	)
	if err != nil {
		if errors.Is(err, store.ErrMinimumAboveLatest) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Warn("minimum supported application version changed",
		"version_code", policy.MinSupportedVersionCode,
		"by", changedBy(r))
	writeJSON(w, http.StatusOK, policy)
}

func changedBy(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("x-changed-by")); value != "" {
		return value
	}
	return "operator"
}
