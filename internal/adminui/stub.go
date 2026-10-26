//go:build noadminui

package adminui

import "net/http"

// Handler 是未构建前端时的占位实现，提示如何生成真实产物。
func Handler() http.Handler {
	body := `<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>AzurPilot 管理后台</title></head>
<body style="font-family: system-ui; max-width: 640px; margin: 80px auto; line-height: 1.8;">
<h1>管理后台前端未构建</h1>
<p>本二进制编译时未包含前端产物。在仓库 <code>web/</code> 目录执行：</p>
<pre><code>npm install
npm run build</code></pre>
<p>然后重新编译服务端即可（或使用 <code>-tags noadminui</code> 显式排除）。</p>
</body></html>`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(body))
	})
}
