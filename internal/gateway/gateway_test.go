package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"
)

// golden 数据由参考实现（AzurLaneServerStatus/checker.py）生成，
// 保证 Go 移植与 Python 协议层逐字节一致。
const (
	goldenRequest  = "000700272200000800"
	goldenResponse = "002f00272300000a12081c20002a08b3a4b9adbcc6bbae3001380a0a14081d20022a0a486f742053657276657230023814"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hexDecode(s)
	if err != nil {
		t.Fatalf("解码 golden 数据失败: %v", err)
	}
	return b
}

func hexDecode(s string) ([]byte, error) {
	out := make([]byte, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		var v byte
		if _, err := fmt.Sscanf(s[i:i+2], "%02x", &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func TestBuildPacketMatchesGolden(t *testing.T) {
	packet := buildPacket(MsgCS10018, encodeVarintField(1, 0))
	if !bytes.Equal(packet, mustHex(t, goldenRequest)) {
		t.Fatalf("10018 请求与 Python golden 不一致:\n got  %x\n want %x", packet, mustHex(t, goldenRequest))
	}
}

func TestDecodeServerlistMatchesGolden(t *testing.T) {
	servers, err := decodeServerlist(mustHex(t, goldenResponse)[7:])
	if err != nil {
		t.Fatalf("解析 golden 响应失败: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("期望 2 个服务器，得到 %d", len(servers))
	}
	// 第一个是 GBK 编码的中文名「长弓计划」。
	if servers[0].ID != 28 || servers[0].Name != "长弓计划" || servers[0].State != 0 || servers[0].TagState != 1 || servers[0].Sort != 10 {
		t.Fatalf("服务器 1 解析错误: %+v", servers[0])
	}
	if servers[0].Status() != "normal" || servers[0].Tag() != "hot" {
		t.Fatalf("服务器 1 状态映射错误: %s %s", servers[0].Status(), servers[0].Tag())
	}
	if servers[1].ID != 29 || servers[1].Name != "Hot Server" || servers[1].Status() != "full" || servers[1].Tag() != "new" {
		t.Fatalf("服务器 2 解析错误: %+v", servers[1])
	}
}

func TestStatusMapping(t *testing.T) {
	cases := map[int]string{0: "normal", 1: "maintenance", 2: "full", 3: "reg_full", 99: "unopened", 7: "unknown"}
	for state, want := range cases {
		if got := (RawServer{State: state}).Status(); got != want {
			t.Fatalf("state %d: got %s want %s", state, got, want)
		}
	}
}

func TestDecodeNameGBKAndUTF8(t *testing.T) {
	if got := decodeName([]byte{0xb3, 0xa4, 0xb9, 0xad}); got != "长弓" {
		t.Fatalf("GBK 解码错误: %q", got)
	}
	if got := decodeName([]byte("hello")); got != "hello" {
		t.Fatalf("UTF-8 解码错误: %q", got)
	}
}

func TestParseResponseRejectsTruncated(t *testing.T) {
	if _, _, err := parseResponse([]byte{0, 1, 2}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("期望 ErrProtocol，得到 %v", err)
	}
}

func TestQueryTCPAgainstMockGateway(t *testing.T) {
	golden := mustHex(t, goldenResponse)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				req := make([]byte, len(goldenRequest)/2)
				if _, err := io.ReadFull(c, req); err != nil {
					return
				}
				c.Write(golden)
			}(conn)
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)
	servers, err := queryTCP(context.Background(), host, portNum, 5*time.Second)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(servers) != 2 || servers[0].Name != "长弓计划" {
		t.Fatalf("结果错误: %+v", servers)
	}
}

func TestQueryHTTPAgainstMockEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "name": "iOS 服", "state": 1, "flag": 0, "sort": 1},
			{"id": 2, "name": "iOS 服 2", "state": 0, "flag": 2, "sort": 2},
		})
	}))
	defer srv.Close()

	servers, err := queryHTTP(context.Background(), srv.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(servers) != 2 || servers[0].Name != "iOS 服" || servers[0].Status() != "maintenance" {
		t.Fatalf("结果错误: %+v", servers)
	}
}

func TestQueryHTTPBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := queryHTTP(context.Background(), srv.URL, 5*time.Second)
	var re *RegionError
	if !errors.As(err, &re) || re.Code != "network_error" {
		t.Fatalf("期望 network_error，得到 %v", err)
	}
}

func TestQueryRegionUnknownRegion(t *testing.T) {
	_, err := QueryRegion(context.Background(), "moon", time.Second)
	var re *RegionError
	if !errors.As(err, &re) || re.Code != "parse_error" {
		t.Fatalf("期望 parse_error，得到 %v", err)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"protocol", fmt.Errorf("wrap: %w", ErrProtocol), "parse_error"},
		{"refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), "refused"},
		{"deadline", fmt.Errorf("read: %w", os.ErrDeadlineExceeded), "timeout"},
		{"other", errors.New("boom"), "network_error"},
	}
	for _, tc := range cases {
		got := classify(tc.err)
		if got.Code != tc.want {
			t.Fatalf("%s: got %s want %s", tc.name, got.Code, tc.want)
		}
	}
}

// timeoutErr 实现 net.Error 的假超时错误。
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassifyNetTimeout(t *testing.T) {
	got := classify(fmt.Errorf("wrap: %w", timeoutErr{}))
	if got.Code != "timeout" {
		t.Fatalf("期望 timeout，得到 %s", got.Code)
	}
}

func TestValidAPIRegion(t *testing.T) {
	for _, r := range APIRegions {
		if !ValidAPIRegion(r) {
			t.Fatalf("%s 应当合法", r)
		}
	}
	if ValidAPIRegion("kr") {
		t.Fatal("kr 不应暴露给 v2 API")
	}
	if ValidAPIRegion("") {
		t.Fatal("空串不合法")
	}
}
