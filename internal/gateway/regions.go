// Package gateway 实现碧蓝航线各区服游戏网关的查询协议层。
//
// 两种协议（移植自 AzurLaneServerStatus 的 checker.py）：
//   - tcp：  连接游戏网关，发送 protobuf 编码的 10018 请求，解析 10019 响应
//   - http： 对 JSON 端点做普通 GET（cn_ios / cn_channel）
//
// 所有网络 IO 都带超时；调用方通过 context 控制取消。
package gateway

import "time"

// RegionConfig 描述一个区服的查询方式。
type RegionConfig struct {
	Name     string
	Protocol string // "tcp" 或 "http"
	Host     string // tcp 专用
	Port     int    // tcp 专用
	URL      string // http 专用
}

// Regions 为全部区服配置。键即区服标识，与 API 中的 region 参数一致。
// kr 存在真实的游戏网关，但 v2 API 不对外暴露（见重构计划 3.3）。
var Regions = map[string]RegionConfig{
	"cn": {
		Name:     "CN Bilibili",
		Protocol: "tcp",
		Host:     "118.178.152.242",
		Port:     80,
	},
	"cn_ios": {
		Name:     "CN iOS",
		Protocol: "http",
		URL:      "http://203.107.54.122/?cmd=load_server?",
	},
	"cn_channel": {
		Name:     "CN Channel",
		Protocol: "http",
		URL:      "http://203.107.54.70/?cmd=load_server?",
	},
	"jp": {
		Name:     "JP",
		Protocol: "tcp",
		Host:     "blhxjploginapi.azurlane.jp",
		Port:     80,
	},
	"en": {
		Name:     "EN",
		Protocol: "tcp",
		Host:     "blhxusgate.yo-star.com",
		Port:     80,
	},
	"kr": {
		Name:     "KR",
		Protocol: "tcp",
		Host:     "bl-kr-gate.xdg.com",
		Port:     80,
	},
	"tw": {
		Name:     "TW",
		Protocol: "tcp",
		Host:     "prod-all-login.azurlane.tw",
		Port:     10080,
	},
}

// APIRegions 是 /api/v2/get/server 允许的区服列表（计划 3.3）。
var APIRegions = []string{"cn", "cn_ios", "cn_channel", "en", "jp", "tw"}

// ValidAPIRegion 报告 region 是否为 v2 API 接受的取值。
func ValidAPIRegion(region string) bool {
	for _, r := range APIRegions {
		if r == region {
			return true
		}
	}
	return false
}

// 状态映射：原始协议 state -> API status 枚举。
var statusMap = map[int]string{
	0:  "normal",
	1:  "maintenance",
	2:  "full",
	3:  "reg_full",
	99: "unopened",
}

// tagMap：原始协议 tag_state -> 标签。
var tagMap = map[int]string{
	1: "hot",
	2: "new",
}

// RawServer 是从网关解析出的单个服务器条目。
type RawServer struct {
	ID       int
	Name     string
	State    int
	TagState int
	Sort     int
}

// Status 返回协议无关的状态枚举。
func (s RawServer) Status() string {
	if status, ok := statusMap[s.State]; ok {
		return status
	}
	return "unknown"
}

// Tag 返回 "hot"、"new" 或空字符串。
func (s RawServer) Tag() string {
	return tagMap[s.TagState]
}

// QueryTimeout 是查询的默认超时；缓存层可以覆盖。
const QueryTimeout = 10 * time.Second
