package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
)

// Discover A at the common API origin without requiring a network route from C
// to A. A 307 preserves the upload method and body for clients following it.
func (s *Server) handleUploadGateway(w http.ResponseWriter, r *http.Request) {
	if id := r.PathValue("id"); id != "" && !uuidPattern.MatchString(id) {
		writeError(w, r, errValidation("upload_id 必须是 UUID", "upload_id"))
		return
	}
	limit := int64(16 << 10)
	if r.Method == http.MethodPut {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 || index >= 1000 {
			writeError(w, r, errValidation("无效文件序号", "index"))
			return
		}
		limit = s.deps.Cfg.StatisticsFileBytes
		if limit == 0 {
			limit = 10 << 20
		}
	}
	if r.ContentLength > limit {
		writeError(w, r, apiError{413, "PAYLOAD_TOO_LARGE", "上传请求超限", map[string]any{}})
		return
	}
	u, err := url.Parse(s.deps.Cfg.LegacyUploadURL)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeError(w, r, errServiceUnavailable())
		return
	}
	target := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
}
