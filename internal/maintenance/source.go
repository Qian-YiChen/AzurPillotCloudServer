// Package maintenance synchronizes CN maintenance windows from the official
// news site's public JSON API. It never infers the next weekly maintenance.
package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ap-cloud-server/internal/store"
	"golang.org/x/text/width"
)

const OfficialPage = "https://game.bilibili.com/blhx/news/"

var (
	cnTime    = time.FixedZone("Asia/Shanghai", 8*3600)
	tags      = regexp.MustCompile(`(?s)<[^>]*>`)
	titleDate = regexp.MustCompile(`(\d{4})年(\d{1,2})月(\d{1,2})日(\d{1,2}):(\d{2})`)
	startTime = regexp.MustCompile(`(?:司令部将于|将于|定于|计划于|维护时间[:为]?)(?:(\d{4})年)?(\d{1,2})月(\d{1,2})日(?:[（(][^）)]*[）)])?(\d{1,2}):(\d{2})`)
	endTime   = regexp.MustCompile(`^(?:~|-|至|到)(次日|翌日)?(?:(?:(\d{4})年)?(\d{1,2})月(\d{1,2})日)?(\d{1,2}):(\d{2})`)
	hours     = regexp.MustCompile(`为期(\d+(?:\.\d+)?)小时`)
)

type article struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Created  string `json:"createTime"`
	Modified string `json:"modifyTime"`
	GameID   int    `json:"gameExtensionId"`
}

type Notice struct {
	Plan  store.Maintenance
	Title string
	URL   string
}

type Source struct {
	client  *http.Client
	baseURL string
}

func NewSource() *Source {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	return &Source{client: &http.Client{Timeout: 5 * time.Second, Transport: transport}, baseURL: "https://api.biligame.com"}
}

func maintenanceTitle(title string) bool {
	return !strings.Contains(title, "不停服") && (strings.Contains(title, "维护公告") ||
		(strings.Contains(title, "公告") && (strings.Contains(title, "停服") || strings.Contains(title, "改建"))))
}

func (s *Source) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AzurPilot-maintenance/1.0)")
	req.Header.Set("Referer", OfficialPage)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("官方公告请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("官方公告 HTTP %d", resp.StatusCode)
	}
	const limit = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if len(body) > limit {
		return fmt.Errorf("官方公告响应超限")
	}
	if err = json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("官方公告 JSON 无效: %w", err)
	}
	return nil
}

func (s *Source) Fetch(ctx context.Context) (Notice, error) {
	defer s.client.CloseIdleConnections()
	var list struct {
		Code *int      `json:"code"`
		Data []article `json:"data"`
	}
	// These are the exact game/position parameters used by the official page.
	if err := s.get(ctx, "/news/list?gameExtensionId=103&positionId=2&typeId=1&pageNum=1&pageSize=20", &list); err != nil {
		return Notice{}, err
	}
	if list.Code == nil || *list.Code != 0 || len(list.Data) == 0 {
		return Notice{}, fmt.Errorf("官方公告列表不可用")
	}
	var candidates []article
	for _, a := range list.Data {
		if a.ID > 0 && maintenanceTitle(a.Title) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return Notice{}, fmt.Errorf("官方列表未找到维护公告，保留上次结果")
	}
	sort.SliceStable(candidates, func(i, j int) bool { return articleTime(candidates[i]).After(articleTime(candidates[j])) })
	latest := candidates[0]
	var detail struct {
		Code *int    `json:"code"`
		Data article `json:"data"`
	}
	if err := s.get(ctx, "/news/"+strconv.Itoa(latest.ID), &detail); err != nil {
		return Notice{}, err
	}
	if detail.Code == nil || *detail.Code != 0 || detail.Data.ID != latest.ID || detail.Data.GameID != 103 || !maintenanceTitle(detail.Data.Title) {
		return Notice{}, fmt.Errorf("官方维护公告详情不匹配")
	}
	// Do not silently fall back to an older article if the latest format changes.
	plan, err := parse(detail.Data)
	if err != nil {
		return Notice{}, err
	}
	return Notice{Plan: plan, Title: detail.Data.Title, URL: OfficialPage + "#news_detail_id=" + strconv.Itoa(latest.ID)}, nil
}

func normalized(s string) string {
	s = html.UnescapeString(tags.ReplaceAllString(s, ""))
	s = width.Fold.String(s)
	s = strings.NewReplacer("～", "~", "—", "-", "–", "-", "：", ":").Replace(s)
	return strings.Join(strings.Fields(s), "")
}

func articleTime(a article) time.Time {
	if match := titleDate.FindStringSubmatch(normalized(a.Title)); match != nil {
		if t, err := date(match[1], match[2], match[3], match[4], match[5]); err == nil {
			return t
		}
	}
	for _, v := range []string{a.Created, a.Modified} {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, cnTime); err == nil {
			return t
		}
	}
	return time.Time{}
}

func date(y, m, d, h, min string) (time.Time, error) {
	parts := []string{y, m, d, h, min}
	n := make([]int, 5)
	for i, p := range parts {
		var err error
		n[i], err = strconv.Atoi(p)
		if err != nil {
			return time.Time{}, fmt.Errorf("维护日期不完整")
		}
	}
	t := time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], 0, 0, cnTime)
	if n[0] < 2000 || t.Year() != n[0] || int(t.Month()) != n[1] || t.Day() != n[2] || t.Hour() != n[3] || t.Minute() != n[4] {
		return time.Time{}, fmt.Errorf("维护日期无效")
	}
	return t, nil
}

func parse(a article) (store.Maintenance, error) {
	text := normalized(a.Content)
	indexes := startTime.FindStringSubmatchIndex(text)
	if indexes == nil {
		return store.Maintenance{}, fmt.Errorf("公告 %d 无法解析维护开始时间", a.ID)
	}
	match := startTime.FindStringSubmatch(text)
	year := match[1]
	if year == "" {
		reference := articleTime(a)
		if reference.IsZero() {
			return store.Maintenance{}, fmt.Errorf("公告 %d 缺少可靠年份", a.ID)
		}
		year = strconv.Itoa(reference.Year())
		// Around New Year, use the year closest to the article's dated context.
		var closest time.Time
		for _, y := range []int{reference.Year() - 1, reference.Year(), reference.Year() + 1} {
			t, err := date(strconv.Itoa(y), match[2], match[3], match[4], match[5])
			if err != nil {
				continue
			}
			if closest.IsZero() || t.Sub(reference).Abs() < closest.Sub(reference).Abs() {
				closest = t
			}
		}
		if !closest.IsZero() {
			year = strconv.Itoa(closest.Year())
		}
	}
	start, err := date(year, match[2], match[3], match[4], match[5])
	if err != nil {
		return store.Maintenance{}, err
	}
	tail := text[indexes[1]:]
	if len(tail) > 1800 {
		tail = tail[:1800]
	}
	if !strings.Contains(tail, "维护") && !strings.Contains(tail, "改造建设") && !strings.Contains(tail, "改建") {
		return store.Maintenance{}, fmt.Errorf("公告 %d 不是停服维护段落", a.ID)
	}
	var end time.Time
	if e := endTime.FindStringSubmatch(tail); e != nil {
		ey, em, ed := year, match[2], match[3]
		if e[3] != "" {
			em, ed = e[3], e[4]
			if e[2] != "" {
				ey = e[2]
			} else if em == "1" && match[2] == "12" {
				ey = strconv.Itoa(start.Year() + 1)
			}
		}
		end, err = date(ey, em, ed, e[5], e[6])
		if err != nil {
			return store.Maintenance{}, err
		}
		if e[1] != "" || (e[3] == "" && !end.After(start)) {
			end = end.AddDate(0, 0, 1)
		}
	} else if d := hours.FindStringSubmatch(tail); d != nil {
		h, parseErr := strconv.ParseFloat(d[1], 64)
		if parseErr != nil || h <= 0 || h > 48 {
			return store.Maintenance{}, fmt.Errorf("维护时长无效")
		}
		end = start.Add(time.Duration(h * float64(time.Hour)))
	} else {
		return store.Maintenance{}, fmt.Errorf("公告 %d 缺少明确维护结束时间或时长", a.ID)
	}
	if !end.After(start) || end.Sub(start) > 48*time.Hour {
		return store.Maintenance{}, fmt.Errorf("维护时间范围无效")
	}
	return store.Maintenance{Server: "cn", Name: "国服维护", StartsAt: start, EndsAt: &end, Source: "official"}, nil
}
