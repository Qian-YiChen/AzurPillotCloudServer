package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// RegionError 是区服查询失败，Code 为稳定错误枚举：
// timeout / refused / network_error / parse_error。
type RegionError struct {
	Code string
	Err  error
}

func (e *RegionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("region query failed (%s): %v", e.Code, e.Err)
	}
	return fmt.Sprintf("region query failed (%s)", e.Code)
}

func (e *RegionError) Unwrap() error { return e.Err }

// 稳定错误码。
var (
	ErrTimeout      = &RegionError{Code: "timeout"}
	ErrRefused      = &RegionError{Code: "refused"}
	ErrNetworkError = &RegionError{Code: "network_error"}
	ErrParseError   = &RegionError{Code: "parse_error"}
)

func classify(err error) *RegionError {
	if err == nil {
		return ErrNetworkError
	}
	var re *RegionError
	if errors.As(err, &re) {
		return re
	}
	if errors.Is(err, ErrProtocol) {
		return &RegionError{Code: "parse_error", Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &RegionError{Code: "timeout", Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &RegionError{Code: "timeout", Err: err}
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return &RegionError{Code: "refused", Err: err}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &RegionError{Code: "parse_error", Err: err}
	}
	return &RegionError{Code: "network_error", Err: err}
}

// decodeGBK 用 GBK 解码字节，失败时以替换字符兜底。
func decodeGBK(value []byte) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(value)
	if err != nil {
		return string(value)
	}
	return string(decoded)
}

// queryTCP 用 10018 请求查询 TCP 网关。
func queryTCP(ctx context.Context, host string, port int, timeout time.Duration) ([]RawServer, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return nil, classify(err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)

	packet := buildPacket(MsgCS10018, encodeServerlistRequest())
	if _, err := conn.Write(packet); err != nil {
		return nil, classify(err)
	}

	// 帧头 2 字节为剩余长度，随后读取剩余部分。
	header := make([]byte, 2)
	if _, err := readFull(conn, header); err != nil {
		return nil, classify(err)
	}
	remaining := int(header[0])<<8 | int(header[1])
	if remaining < 5 || remaining > 4*1024*1024 {
		return nil, classify(ErrProtocol)
	}
	rest := make([]byte, remaining)
	if _, err := readFull(conn, rest); err != nil {
		return nil, classify(err)
	}

	msgID, body, err := parseResponse(append(header, rest...))
	if err != nil {
		return nil, classify(err)
	}
	if msgID != MsgSC10019 {
		return nil, classify(fmt.Errorf("unexpected message id %d: %w", msgID, ErrProtocol))
	}

	servers, err := decodeServerlist(body)
	if err != nil {
		return nil, classify(err)
	}
	return servers, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// queryHTTP 查询 HTTP JSON 端点（cn_ios / cn_channel）。
// 显式禁用代理，与参考实现保持一致，避免环境代理影响网关探测。
func queryHTTP(ctx context.Context, rawURL string, timeout time.Duration) ([]RawServer, error) {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          1,
		IdleConnTimeout:       0,
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, classify(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classify(fmt.Errorf("http status %d", resp.StatusCode))
	}

	var items []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		State int    `json:"state"`
		Flag  int    `json:"flag"`
		Sort  int    `json:"sort"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&items); err != nil {
		return nil, classify(fmt.Errorf("%w: JSON 解析失败: %v", ErrProtocol, err))
	}

	servers := make([]RawServer, 0, len(items))
	for _, item := range items {
		servers = append(servers, RawServer{
			ID:       item.ID,
			Name:     item.Name,
			State:    item.State,
			TagState: item.Flag,
			Sort:     item.Sort,
		})
	}
	return servers, nil
}

// QueryRegion 按 key 查询一个区服。
// 未知 key 返回 parse_error；查询失败返回带稳定错误码的 RegionError。
func QueryRegion(ctx context.Context, key string, timeout time.Duration) ([]RawServer, error) {
	config, ok := Regions[key]
	if !ok {
		return nil, &RegionError{Code: "parse_error", Err: fmt.Errorf("unknown region %q", key)}
	}
	if config.Protocol == "tcp" {
		return queryTCP(ctx, config.Host, config.Port, timeout)
	}
	return queryHTTP(ctx, config.URL, timeout)
}
