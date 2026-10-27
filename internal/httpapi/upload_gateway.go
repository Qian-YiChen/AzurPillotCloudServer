package httpapi

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

// Expose A's resumable protocol at the common API origin. ReverseProxy streams
// each file directly to A; C neither buffers the complete body nor stores images.
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
	target := &url.URL{Scheme: u.Scheme, Host: u.Host}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.SetXForwarded()
			p.Out.Header.Del("Authorization")
			p.Out.Header.Del("Cookie")
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			var size *http.MaxBytesError
			if errors.As(err, &size) {
				writeError(w, r, apiError{413, "PAYLOAD_TOO_LARGE", "上传请求超限", map[string]any{}})
			} else {
				writeError(w, r, errSourceUnavailable())
			}
		},
	}
	proxy.ServeHTTP(w, r)
}
