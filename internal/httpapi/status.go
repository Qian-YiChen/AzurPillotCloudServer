package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"ap-cloud-server/internal/gateway"
)

// serverResponse 是 GET /api/v2/get/server 的响应结构（计划 3.3）。
type serverResponse struct {
	SchemaVersion int        `json:"schema_version"`
	UpdatedAt     string     `json:"updated_at"`
	Region        string     `json:"region"`
	Server        serverInfo `json:"server"`
}

type serverInfo struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// maintenanceResponse 是 GET /api/v2/get/maintenance 的响应结构（计划 3.4）。
type maintenanceResponse struct {
	SchemaVersion int              `json:"schema_version"`
	UpdatedAt     string           `json:"updated_at"`
	Server        string           `json:"server"`
	Maintenance   *maintenanceInfo `json:"maintenance"`
}

type maintenanceInfo struct {
	Name     string  `json:"name"`
	StartsAt string  `json:"starts_at"`
	EndsAt   *string `json:"ends_at"`
}

// handleGetServer 处理 GET /api/v2/get/server?region=cn&server_id=28。
func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, errMethodNotAllowed(http.MethodGet))
		return
	}

	region := r.URL.Query().Get("region")
	if !gateway.ValidAPIRegion(region) {
		writeError(w, r, errValidation("region 取值不合法", "region"))
		return
	}
	serverID, err := strconv.Atoi(r.URL.Query().Get("server_id"))
	if err != nil || serverID <= 0 {
		writeError(w, r, errValidation("server_id 必须为正整数", "server_id"))
		return
	}

	entry, _, err := s.cache.Get(r.Context(), region)
	if err != nil {
		// 网关不可用且无缓存 → 503。
		writeError(w, r, errSourceUnavailable())
		return
	}

	for _, srv := range entry.Servers {
		if srv.ID == serverID {
			writeJSON(w, r, http.StatusOK, serverResponse{
				SchemaVersion: schemaVersion,
				UpdatedAt:     s.now().UTC().Format(time.RFC3339),
				Region:        region,
				Server: serverInfo{
					ID:     srv.ID,
					Name:   srv.Name,
					Status: srv.Status(),
				},
			})
			return
		}
	}
	writeError(w, r, errServerNotFound())
}

// handleGetMaintenance 处理 GET /api/v2/get/maintenance?server=cn。
func (s *Server) handleGetMaintenance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, errMethodNotAllowed(http.MethodGet))
		return
	}

	server := r.URL.Query().Get("server")
	switch server {
	case "cn", "en", "jp", "tw":
	default:
		writeError(w, r, errValidation("server 取值不合法", "server"))
		return
	}

	m, err := s.store.LatestMaintenance(r.Context(), server)
	if err != nil {
		// 数据源失败不能用 maintenance: null 冒充查询成功。
		s.logger.Error("查询维护计划失败", "err", err)
		writeError(w, r, errServiceUnavailable())
		return
	}

	resp := maintenanceResponse{
		SchemaVersion: schemaVersion,
		UpdatedAt:     s.now().UTC().Format(time.RFC3339),
		Server:        server,
	}
	if m != nil {
		info := &maintenanceInfo{
			Name:     m.Name,
			StartsAt: m.StartsAt.UTC().Format(time.RFC3339),
		}
		if m.EndsAt != nil {
			ends := m.EndsAt.UTC().Format(time.RFC3339)
			info.EndsAt = &ends
		}
		resp.Maintenance = info
	}
	writeJSON(w, r, http.StatusOK, resp)
}
