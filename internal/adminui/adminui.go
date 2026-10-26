//go:build !noadminui

// Package adminui 内嵌管理后台前端（React 构建产物）。
//
// web/ 目录执行 `npm run build` 后产物输出到本包 dist/，
// 由 go:embed 打进二进制并挂载到 /admin。
// 未构建前端时使用 noadminui 构建标签编译 stub 版本。
package adminui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler 返回挂在 /admin/ 前缀下的管理后台处理器。
// 静态资源命中文件；其余路径回退 index.html（SPA）。
// 不用 http.FileServer：它按完整 URL 路径映射文件系统且会对
// 显式 index.html 做 301 规范化，与前缀挂载相性差。
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed 保证 dist 存在，仅在编码错误时触发。
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, "/admin")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			rel = "index.html"
		}

		data, statErr := fs.ReadFile(sub, rel)
		if statErr != nil {
			// SPA 回退：未知路径返回入口页。
			data, err = fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "admin UI not built", http.StatusInternalServerError)
				return
			}
			rel = "index.html"
		}

		if strings.HasPrefix(rel, "assets/") {
			// 带内容哈希的静态资源可长期缓存；入口页保持即时更新。
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		ct := mime.TypeByExtension(path.Ext(rel))
		if ct != "" && strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "charset") {
			ct += "; charset=utf-8"
		}
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(data)
	})
}
