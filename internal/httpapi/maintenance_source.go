package httpapi

import (
	"errors"
	"net/http"

	"ap-cloud-server/internal/maintenance"
)

func (s *Server) handleAdminMaintenanceSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, r, errMethodNotAllowed("GET, POST"))
		return
	}
	if s.deps.Maintenance == nil {
		writeError(w, r, errServiceUnavailable())
		return
	}
	if r.Method == http.MethodPost {
		if err := s.deps.Maintenance.Sync(r.Context()); err != nil {
			if errors.Is(err, maintenance.ErrBusy) {
				writeError(w, r, errRequestConflict(err.Error()))
			} else {
				writeError(w, r, errSourceUnavailable())
			}
			return
		}
	}
	state, err := s.deps.Maintenance.Status(r.Context())
	if err != nil {
		writeError(w, r, errServiceUnavailable())
		return
	}
	writeJSON(w, r, http.StatusOK, state)
}
