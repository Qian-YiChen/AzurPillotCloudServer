package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ap-cloud-server/internal/publish"
	"ap-cloud-server/internal/store"
)

// adminAuth 校验管理接口的 Bearer Token。未配置 ADMIN_TOKEN 时整体关闭。
func (s *Server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			writeError(w, r, errForbidden())
			return
		}
		auth := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || token == "" ||
			subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken)) != 1 {
			writeError(w, r, errUnauthorized())
			return
		}
		next(w, r)
	}
}

type announcementRequest struct {
	Title   string  `json:"title"`
	Content *string `json:"content"`
	URL     *string `json:"url"`
}

type updatePolicyRequest struct {
	ForceUpdate *bool `json:"force_update"`
}

type maintenanceRequest struct {
	Name     string  `json:"name"`
	StartsAt string  `json:"starts_at"`
	EndsAt   *string `json:"ends_at"`
}

// handleAdminAnnouncement 处理 GET/PUT/DELETE /api/v1/admin/announcement。
func (s *Server) handleAdminAnnouncement(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a, err := s.store.LatestAnnouncement(r.Context())
		if err != nil {
			writeError(w, r, errInternal())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"announcement": a})
	case http.MethodPut:
		var req announcementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, r, errInvalidJSON())
			return
		}
		if strings.TrimSpace(req.Title) == "" {
			writeError(w, r, errValidation("title 不能为空", "title"))
			return
		}
		contentEmpty := req.Content == nil || strings.TrimSpace(*req.Content) == ""
		urlEmpty := req.URL == nil || strings.TrimSpace(*req.URL) == ""
		if contentEmpty && urlEmpty {
			writeError(w, r, errValidation("content 与 url 至少一项有内容", "content"))
			return
		}
		now := s.now()
		id, err := s.store.GenerateAnnouncementID(r.Context(), now)
		if err != nil {
			writeError(w, r, errInternal())
			return
		}
		a := store.Announcement{ID: id, Title: req.Title, Content: req.Content, URL: req.URL}
		if err := s.store.SetAnnouncement(r.Context(), a); err != nil {
			s.logger.Error("保存公告失败", "err", err)
			writeError(w, r, errInternal())
			return
		}
		stored, err := s.store.LatestAnnouncement(r.Context())
		if err != nil || stored == nil {
			writeError(w, r, errInternal())
			return
		}
		res := s.publishNotice(r.Context())
		writeJSON(w, r, http.StatusOK, map[string]any{
			"announcement": stored, "publish": publishResultJSON(res),
		})
	case http.MethodDelete:
		if err := s.store.WithdrawAnnouncement(r.Context()); err != nil {
			s.logger.Error("撤回公告失败", "err", err)
			writeError(w, r, errInternal())
			return
		}
		res := s.publishNotice(r.Context())
		writeJSON(w, r, http.StatusOK, map[string]any{"publish": publishResultJSON(res)})
	default:
		writeError(w, r, errMethodNotAllowed("GET, PUT, DELETE"))
	}
}

// handleAdminUpdatePolicy 处理 GET/PUT /api/v1/admin/update-policy。
func (s *Server) handleAdminUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		force, err := s.store.GetUpdatePolicy(r.Context())
		if err != nil {
			writeError(w, r, errInternal())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"force_update": force})
	case http.MethodPut:
		var req updatePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, r, errInvalidJSON())
			return
		}
		if req.ForceUpdate == nil {
			writeError(w, r, errValidation("force_update 必须为布尔值", "force_update"))
			return
		}
		if err := s.store.SetUpdatePolicy(r.Context(), *req.ForceUpdate); err != nil {
			s.logger.Error("保存更新策略失败", "err", err)
			writeError(w, r, errInternal())
			return
		}
		res := s.publishUpdate(r.Context())
		writeJSON(w, r, http.StatusOK, map[string]any{
			"force_update": *req.ForceUpdate, "publish": publishResultJSON(res),
		})
	default:
		writeError(w, r, errMethodNotAllowed("GET, PUT"))
	}
}

// handleAdminMaintenance 处理 GET /api/v1/admin/maintenance、
// PUT/DELETE /api/v1/admin/maintenance/{server}。
func (s *Server) handleAdminMaintenance(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListMaintenances(r.Context())
		if err != nil {
			writeError(w, r, errInternal())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"maintenances": list})
	case http.MethodPut:
		server := r.PathValue("server")
		switch server {
		case "cn", "en", "jp", "tw":
		default:
			writeError(w, r, errValidation("server 取值不合法", "server"))
			return
		}
		var req maintenanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, r, errInvalidJSON())
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, r, errValidation("name 不能为空", "name"))
			return
		}
		startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
		if err != nil {
			writeError(w, r, errValidation("starts_at 必须是带时区的 RFC 3339 时间", "starts_at"))
			return
		}
		var endsAt *time.Time
		if req.EndsAt != nil && *req.EndsAt != "" {
			parsed, err := time.Parse(time.RFC3339, *req.EndsAt)
			if err != nil {
				writeError(w, r, errValidation("ends_at 必须是带时区的 RFC 3339 时间", "ends_at"))
				return
			}
			if parsed.Before(startsAt) {
				writeError(w, r, errValidation("ends_at 不得早于 starts_at", "ends_at"))
				return
			}
			endsAt = &parsed
		}
		if err := s.store.UpsertMaintenance(r.Context(), store.Maintenance{
			Server: server, Name: req.Name, StartsAt: startsAt, EndsAt: endsAt,
		}); err != nil {
			s.logger.Error("保存维护计划失败", "err", err)
			writeError(w, r, errInternal())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"saved": true})
	case http.MethodDelete:
		server := r.PathValue("server")
		switch server {
		case "cn", "en", "jp", "tw":
		default:
			writeError(w, r, errValidation("server 取值不合法", "server"))
			return
		}
		deleted, err := s.store.DeleteMaintenance(r.Context(), server)
		if err != nil {
			writeError(w, r, errInternal())
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{"deleted": deleted})
	default:
		writeError(w, r, errMethodNotAllowed("GET, PUT, DELETE"))
	}
}

// handleAdminPublishStatus 返回每个 Worker 最近一次发布状态。
func (s *Server) handleAdminPublishStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, errMethodNotAllowed(http.MethodGet))
		return
	}
	latest, err := s.store.LatestPublishByWorker(r.Context())
	if err != nil {
		writeError(w, r, errInternal())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"publish_history": latest})
}

// publishNotice 触发公告 Worker 的发布。
func (s *Server) publishNotice(ctx context.Context) publish.Result {
	a, err := s.store.LatestAnnouncement(ctx)
	if err != nil {
		return publish.Result{Error: "读取公告失败: " + err.Error()}
	}
	content, err := publish.RenderNotice(a, s.now())
	if err != nil {
		return publish.Result{Error: err.Error()}
	}
	noticePath, _ := publish.Paths(s.publishDir)
	return s.publisher.Submit(ctx, publish.Job{
		Worker:    publish.NoticeWorker,
		Path:      "/api/v2/get/notice/data.json",
		LocalPath: noticePath,
		Content:   content,
		PublicURL: s.noticePublicURL,
	}, s.publishTimeout)
}

// publishUpdate 触发更新策略 Worker 的发布。
func (s *Server) publishUpdate(ctx context.Context) publish.Result {
	force, err := s.store.GetUpdatePolicy(ctx)
	if err != nil {
		return publish.Result{Error: "读取更新策略失败: " + err.Error()}
	}
	content, err := publish.RenderUpdate(force, s.now())
	if err != nil {
		return publish.Result{Error: err.Error()}
	}
	_, updatePath := publish.Paths(s.publishDir)
	return s.publisher.Submit(ctx, publish.Job{
		Worker:    publish.UpdateWorker,
		Path:      "/api/v2/get/update/data.json",
		LocalPath: updatePath,
		Content:   content,
		PublicURL: s.updatePublicURL,
	}, s.publishTimeout)
}

func publishResultJSON(res publish.Result) map[string]any {
	return map[string]any{
		"success": res.Success,
		"skipped": res.Skipped,
		"error":   res.Error,
	}
}
