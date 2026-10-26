// Package publish 负责公告与更新策略静态文件的生成与发布。
//
// 发布流程（重构计划 4）：
//  1. 依据数据库当前状态渲染完整静态 JSON 文件；
//  2. 原子写入本地发布目录（Worker Static Assets 目录）；
//  3. 通过 Cloudflare REST API 上传资产并部署（凭据只在服务器端）；
//  4. 从公网地址读取确认内容一致后才记录发布成功；
//  5. 任一步失败都保留上一版本（部署本身是原子操作）。
//
// 每个 Worker 的发布严格串行执行。
package publish

import (
	"os"
	"path/filepath"
)

// Paths 返回本地发布目录内两个静态文件路径。
func Paths(publishDir string) (noticePath, updatePath string) {
	return filepath.Join(publishDir, "api", "v2", "get", "notice", "data.json"),
		filepath.Join(publishDir, "api", "v2", "get", "update", "data.json")
}

// writeAtomic 先写临时文件再改名，避免读取到半写的静态文件。
func writeAtomic(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".data.json-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
