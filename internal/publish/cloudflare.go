package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// Client 是 Cloudflare Workers 静态资产发布客户端。
// 走官方 Direct Upload REST 流程：
//  1. 创建资产上传会话（manifest 注册，返回 jwt 与需要上传的桶）；
//  2. 上传有变化的文件（base64 multipart）；
//  3. 用完成令牌部署 Worker（原子替换，失败保留上一版本）。
type Client struct {
	accountID string
	apiToken  string
	baseURL   string
	http      *http.Client
}

// NewClient 创建发布客户端。baseURL 供测试注入模拟 API。
func NewClient(accountID, apiToken string, timeout time.Duration, baseURL string) *Client {
	return &Client{
		accountID: accountID,
		apiToken:  apiToken,
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		http:      &http.Client{Timeout: timeout},
	}
}

// AssetFile 是一个待发布的静态文件。
type AssetFile struct {
	// Path 是资产在 Worker 内的完整 URL 路径，如 /api/v2/get/notice/data.json。
	Path string
	// Content 是文件内容。
	Content []byte
	// ContentType 是提供该文件时的 Content-Type。
	ContentType string
}

// assetHash 计算 Cloudflare 资产哈希：内容 SHA-256 的十六进制。
func assetHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

type cfResponse struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []cfError       `json:"errors"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type uploadSessionResult struct {
	JWT     string     `json:"jwt"`
	Buckets [][]string `json:"buckets"`
}

type uploadCompletion struct {
	JWT string `json:"jwt"`
}

// api 调用 Cloudflare API 并解析标准信封。
func (c *Client) api(ctx context.Context, method, path string, contentType string, body io.Reader) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 Cloudflare 响应失败: %w", err)
	}
	var envelope cfResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("cloudflare 返回非 JSON（HTTP %d）: %s", resp.StatusCode, truncate(raw, 200))
	}
	if !envelope.Success || resp.StatusCode >= 400 {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if len(envelope.Errors) > 0 {
			msg = fmt.Sprintf("HTTP %d: code %d %s", resp.StatusCode, envelope.Errors[0].Code, envelope.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare API 调用失败: %s", msg)
	}
	return envelope.Result, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}

// PublishWorker 发布一组静态资产并原子部署 Worker。
func (c *Client) PublishWorker(ctx context.Context, scriptName string, mainModule []byte, files []AssetFile) error {
	// 1. 注册 manifest，创建上传会话。
	manifest := map[string]manifestEntry{}
	for _, f := range files {
		manifest[f.Path] = manifestEntry{Hash: assetHash(f.Content), Size: len(f.Content)}
	}
	manifestJSON, err := json.Marshal(map[string]any{"manifest": manifest})
	if err != nil {
		return err
	}
	result, err := c.api(ctx, http.MethodPost,
		fmt.Sprintf("/client/v4/accounts/%s/workers/scripts/%s/assets-upload-session", c.accountID, scriptName),
		"application/json", bytes.NewReader(manifestJSON))
	if err != nil {
		return fmt.Errorf("创建上传会话失败: %w", err)
	}
	var session uploadSessionResult
	if err := json.Unmarshal(result, &session); err != nil {
		return fmt.Errorf("解析上传会话失败: %w", err)
	}
	if session.JWT == "" {
		return fmt.Errorf("上传会话未返回 jwt")
	}

	// 2. 上传有变化的文件；桶为空说明内容未变化，jwt 已是完成令牌。
	completionToken := session.JWT
	if len(session.Buckets) > 0 {
		token, err := c.uploadAssets(ctx, session.JWT, files)
		if err != nil {
			return err
		}
		completionToken = token
	}

	// 3. 用完成令牌原子部署 Worker（模块脚本 + 静态资产绑定）。
	return c.deployWorker(ctx, scriptName, mainModule, completionToken)
}

type manifestEntry struct {
	Hash string `json:"hash"`
	Size int    `json:"size"`
}

func (c *Client) uploadAssets(ctx context.Context, jwt string, files []AssetFile) (string, error) {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for _, f := range files {
		if err := createFormPart(form, assetHash(f.Content), f.Path, f.ContentType, f.Content); err != nil {
			return "", err
		}
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	resp, err := c.uploadRaw(ctx, jwt, body, form.FormDataContentType())
	if err != nil {
		return "", err
	}

	var completion uploadCompletion
	// 上传端点直接返回 {"jwt": "..."}。
	if err := json.Unmarshal(resp, &completion); err == nil && completion.JWT != "" {
		return completion.JWT, nil
	}
	// 兼容标准信封 {"result": {"jwt": "..."}}。
	var envelope cfResponse
	if err := json.Unmarshal(resp, &envelope); err == nil && len(envelope.Result) > 0 {
		var inner uploadCompletion
		if err := json.Unmarshal(envelope.Result, &inner); err == nil && inner.JWT != "" {
			return inner.JWT, nil
		}
	}
	return "", fmt.Errorf("资产上传完成但未返回完成令牌: %s", truncate(resp, 200))
}

// createFormPart 写入一个 base64 编码的文件部件，
// 并显式设置提供该文件时的 Content-Type。
func createFormPart(form *multipart.Writer, hash, path, contentType string, content []byte) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, hash, path))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	} else {
		header.Set("Content-Type", "application/octet-stream")
	}
	part, err := form.CreatePart(header)
	if err != nil {
		return err
	}
	enc := base64.NewEncoder(base64.StdEncoding, part)
	if _, err := enc.Write(content); err != nil {
		return err
	}
	return enc.Close()
}

func (c *Client) uploadRaw(ctx context.Context, jwt string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/client/v4/accounts/"+c.accountID+"/workers/assets/upload?base64=true", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("上传资产失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode >= 400 {
		return nil, fmt.Errorf("上传资产失败: HTTP %d: %s", resp.StatusCode, truncate(raw, 200))
	}
	return raw, nil
}

// deployMetadata 是部署脚本的 multipart metadata 部分。
type deployMetadata struct {
	MainModule        string          `json:"main_module"`
	Assets            deployAssets    `json:"assets"`
	Bindings          []deployBinding `json:"bindings,omitempty"`
	CompatibilityDate string          `json:"compatibility_date"`
}

type deployAssets struct {
	JWT    string       `json:"jwt"`
	Config assetsConfig `json:"config"`
}

type assetsConfig struct {
	HTMLHandling     string `json:"html_handling"`
	NotFoundHandling string `json:"not_found_handling"`
}

type deployBinding struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (c *Client) deployWorker(ctx context.Context, scriptName string, mainModule []byte, completionToken string) error {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)

	metaJSON, err := json.Marshal(deployMetadata{
		MainModule: "main.js",
		Assets: deployAssets{
			JWT: completionToken,
			Config: assetsConfig{
				HTMLHandling:     "auto-trailing-slash",
				NotFoundHandling: "none",
			},
		},
		Bindings: []deployBinding{{Name: "ASSETS", Type: "assets"}},
		// 静态资源型 Worker 只依赖基础运行时，固定较新的兼容日期。
		CompatibilityDate: "2026-01-01",
	})
	if err != nil {
		return err
	}
	metaPart, err := form.CreateFormFile("metadata", "metadata.json")
	if err != nil {
		return err
	}
	if _, err := metaPart.Write(metaJSON); err != nil {
		return err
	}
	modulePart, err := form.CreateFormFile("main.js", "main.js")
	if err != nil {
		return err
	}
	if _, err := modulePart.Write(mainModule); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}

	if _, err := c.api(ctx, http.MethodPut,
		fmt.Sprintf("/client/v4/accounts/%s/workers/scripts/%s", c.accountID, scriptName),
		form.FormDataContentType(), body); err != nil {
		return fmt.Errorf("部署 Worker 失败: %w", err)
	}
	return nil
}
