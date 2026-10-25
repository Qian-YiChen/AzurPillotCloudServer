package publish

import (
	"encoding/json"
	"fmt"
	"time"

	"ap-cloud-server/internal/store"
)

const schemaVersion = 2

// noticeDoc 是 /api/v2/get/notice/data.json 的文档结构（计划 3.1）。
type noticeDoc struct {
	SchemaVersion int                 `json:"schema_version"`
	UpdatedAt     string              `json:"updated_at"`
	Announcement  *noticeAnnouncement `json:"announcement"`
}

type noticeAnnouncement struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Content *string `json:"content"`
	URL     *string `json:"url"`
}

// updateDoc 是 /api/v2/get/update/data.json 的文档结构（计划 3.2）。
type updateDoc struct {
	SchemaVersion int    `json:"schema_version"`
	UpdatedAt     string `json:"updated_at"`
	ForceUpdate   bool   `json:"force_update"`
}

// RenderNotice 渲染公告文档。a 为 nil 时发布 announcement: null（空公告），
// 撤回也通过发布空文档表达，不删除文件。
func RenderNotice(a *store.Announcement, updatedAt time.Time) ([]byte, error) {
	doc := noticeDoc{
		SchemaVersion: schemaVersion,
		UpdatedAt:     updatedAt.UTC().Format(time.RFC3339),
		Announcement:  nil,
	}
	if a != nil {
		doc.Announcement = &noticeAnnouncement{
			ID:      a.ID,
			Title:   a.Title,
			Content: a.Content,
			URL:     a.URL,
		}
	}
	return marshal(doc)
}

// RenderUpdate 渲染更新策略文档。
func RenderUpdate(forceUpdate bool, updatedAt time.Time) ([]byte, error) {
	return marshal(updateDoc{
		SchemaVersion: schemaVersion,
		UpdatedAt:     updatedAt.UTC().Format(time.RFC3339),
		ForceUpdate:   forceUpdate,
	})
}

func marshal(v any) ([]byte, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("渲染静态文档失败: %w", err)
	}
	// 追加换行，保持文件以换行结尾。
	return append(buf, '\n'), nil
}
