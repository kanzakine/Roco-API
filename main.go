package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ============================================================
// 配置
// ============================================================

var (
	cfgPort      = ":8008"
	cfgTargetURL = "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0"
	cfgInterval  = 3
	cfgChanUID   string
	cfgChanKey   string
)

func loadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if p, ok := m["port"].(string); ok {
		cfgPort = p
	}
	if u, ok := m["target_url"].(string); ok {
		cfgTargetURL = u
	}
	if n, ok := m["interval"].(float64); ok && n > 0 {
		cfgInterval = int(n)
	}
	if u, ok := m["chan_uid"].(string); ok {
		cfgChanUID = u
	}
	if k, ok := m["chan_key"].(string); ok {
		cfgChanKey = k
	}
	// 环境变量覆盖
	if e := os.Getenv("CHAN_UID"); e != "" {
		cfgChanUID = e
	}
	if e := os.Getenv("CHAN_KEY"); e != "" {
		cfgChanKey = e
	}
	fmt.Printf("[配置] 配置加载完成 (端口: %s, 爬取间隔: %d分钟)\n", cfgPort, cfgInterval)
}

func chanEnabled() bool {
	return cfgChanUID != "" && cfgChanKey != ""
}

// ============================================================
// 数据模型
// ============================================================

type Product struct {
	Name      string `json:"name"`
	Price     string `json:"price"`
	Limit     string `json:"limit"`
	Category  string `json:"category"`
	Desc      string `json:"desc"`
	ImageURL  string `json:"image_url"`
	IsOnSale  bool   `json:"is_on_sale"`
	RemainStr string `json:"remain"`
	SlotLabel string `json:"slot_label"`
}

type CrawlResult struct {
	Products    []Product `json:"products"`
	OnSaleCount int       `json:"on_sale_count"`
	TotalCount  int       `json:"total_count"`
	UpdatedAt   string    `json:"updated_at"`
}

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ============================================================
// 推送
// ============================================================

func sendPush(title, desp string) {
	if !chanEnabled() {
		return
	}
	u := fmt.Sprintf("https://%s.push.ft07.com/send/%s.send", cfgChanUID, cfgChanKey)
	body, _ := json.Marshal(map[string]string{"title": title, "desp": desp, "tags": "Roco-API"})
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Post(u, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[错误] 推送请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	var r struct{ Code int }
	json.NewDecoder(resp.Body).Decode(&r)
	if r.Code == 0 {
		log.Println("[完成] 推送成功")
	} else {
		log.Println("[警告] 推送返回异常")
	}
}

// ============================================================
// 过去商品
// ============================================================

type PastItem struct{ Name, Slot, Price string }

var (
	pastProducts []PastItem
	pastMu       sync.Mutex
)

func addPastItems(products []Product) {
	pastMu.Lock()
	defer pastMu.Unlock()
	seen := map[string]bool{}
	for _, r := range pastProducts {
		seen[r.Name] = true
	}
	for _, p := range products {
		if !p.IsOnSale && p.SlotLabel != "" && !seen[p.Name] {
			pastProducts = append(pastProducts, PastItem{Name: p.Name, Slot: p.SlotLabel, Price: p.Price})
			seen[p.Name] = true
		}
	}
}

// ============================================================
// 爬虫
// ============================================================

var (
	cache      CrawlResult
	cacheMutex sync.RWMutex
)

func doCrawl() {
	resp, err := http.Get(cfgTargetURL)
	if err != nil {
		log.Printf("[错误] 请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("[错误] 解析 HTML 失败: %v", err)
		return
	}

	now := time.Now().UTC()
	var products []Product

	doc.Find("li[data-time]").Each(func(i int, s *goquery.Selection) {
		dt, _ := s.Attr("data-time")
		oc, _ := s.Attr("onclick")

		// 从 onclick 解析
		_ = oc
		cat, desc := "", ""
		re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
		m := re.FindStringSubmatch(oc)
		if len(m) >= 5 {
			cat = m[3]
			desc = m[4]
		}

		price := strings.TrimSpace(s.Find(".shop_price").Text())
		price = strings.Replace(price, "价格：", "", 1)

		p := Product{
			Name:     strings.TrimSpace(s.Find(".shop_name").Text()),
			Price:    price,
			Limit:    strings.TrimSpace(s.Find(".gitem em").First().Text()),
			Category: cat,
			Desc:     desc,
		}
		if p.Name == "" {
			return
		}

		img, _ := s.Find(".gitem img").Attr("src")
		p.ImageURL = img

		if dt != "" {
			if ts, err := strconv.ParseInt(dt, 10, 64); err == nil {
				end := time.Unix(ts, 0)
				start := end.Add(-4 * time.Hour)
				p.IsOnSale = now.After(start) && now.Before(end)
				h := end.In(time.FixedZone("CST", 8*3600)).Hour()
				switch {
				case h >= 8 && h < 12:
					p.SlotLabel = "08:00-12:00"
				case h >= 12 && h < 16:
					p.SlotLabel = "12:00-16:00"
				case h >= 16 && h < 20:
					p.SlotLabel = "16:00-20:00"
				default:
					p.SlotLabel = "20:00-24:00"
				}
				if p.IsOnSale {
					d := end.Sub(now)
					p.RemainStr = fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
				}
			}
		}

		products = append(products, p)
	})

	onsale := 0
	for _, p := range products {
		if p.IsOnSale {
			onsale++
		}
	}

	cacheMutex.Lock()
	cache = CrawlResult{
		Products: products, OnSaleCount: onsale,
		TotalCount: len(products), UpdatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	cacheMutex.Unlock()

	fmt.Printf("[完成] 爬取完成: %d 件商品, %d 件在售中\n", len(products), onsale)

	addPastItems(products)

	if chanEnabled() && onsale > 0 {
		sendPush(fmt.Sprintf("🏪 远行商人更新 · %d 件在售", onsale), buildMsg(products))
	}
}

func startCron() {
	for range time.NewTicker(time.Duration(cfgInterval) * time.Minute).C {
		fmt.Printf("[定时] 爬取中...\n")
		doCrawl()
	}
}

// ============================================================
// 消息构建
// ============================================================

func buildMsg(products []Product) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 🕐 %s\n\n", curSlot()))

	for _, p := range products {
		if !p.IsOnSale {
			continue
		}
		b.WriteString(fmt.Sprintf("**%s**  💰 %s", p.Name, p.Price))
		if p.Limit != "" {
			b.WriteString(fmt.Sprintf("  📦 %s", p.Limit))
		}
		if p.Category != "" {
			b.WriteString(fmt.Sprintf("  📂 %s", p.Category))
		}
		if p.RemainStr != "" {
			b.WriteString(fmt.Sprintf("  ⏱ %s", p.RemainStr))
		}
		b.WriteString("\n\n")
	}

	pastMu.Lock()
	if len(pastProducts) > 0 {
		sorted := make([]PastItem, len(pastProducts))
		copy(sorted, pastProducts)
		order := map[string]int{"08:00-12:00": 1, "12:00-16:00": 2, "16:00-20:00": 3, "20:00-24:00": 4}
		sort.Slice(sorted, func(i, j int) bool { return order[sorted[i].Slot] < order[sorted[j].Slot] })
		b.WriteString("---\n### 📜 今日过往商品\n\n")
		for _, r := range sorted {
			b.WriteString(fmt.Sprintf("- %s（%s %s）\n", r.Name, r.Slot, r.Price))
		}
	}
	pastMu.Unlock()

	b.WriteString(fmt.Sprintf("---\n📡 [查看完整页面](http://localhost%s)\n", cfgPort))
	return b.String()
}

func curSlot() string {
	h := time.Now().In(time.FixedZone("CST", 8*3600)).Hour()
	switch {
	case h >= 8 && h < 12:
		return "08:00-12:00"
	case h >= 12 && h < 16:
		return "12:00-16:00"
	case h >= 16 && h < 20:
		return "16:00-20:00"
	default:
		return "20:00-24:00"
	}
}

// ============================================================
// 主函数
// ============================================================

func main() {
	loadConfig("config.json")
	if chanEnabled() {
		fmt.Println("[通知] Server酱 推送已启用")
	} else {
		fmt.Println("[信息] Server酱 推送未配置（如需启用，请设置 CHAN_UID 和 CHAN_KEY 环境变量）")
	}

	fmt.Println("[任务] 首次爬取远行商人数据...")
	doCrawl()
	go startCron()

	http.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		cacheMutex.RLock()
		defer cacheMutex.RUnlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(APIResponse{Code: 200, Message: "success", Data: cache})
	})
	http.HandleFunc("/api/onsale", func(w http.ResponseWriter, r *http.Request) {
		cacheMutex.RLock()
		defer cacheMutex.RUnlock()
		var onsale []Product
		for _, p := range cache.Products {
			if p.IsOnSale {
				onsale = append(onsale, p)
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(APIResponse{Code: 200, Message: "success", Data: map[string]interface{}{
			"products": onsale, "on_sale_count": len(onsale),
			"total_count": cache.TotalCount, "updated_at": cache.UpdatedAt,
		}})
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cacheMutex.RLock()
		result := cache
		cacheMutex.RUnlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body><h1>🏪 远行商人监控</h1><p>商品 %d | 在售 %d | 更新 %s</p><p>/api/products | /api/onsale</p></body></html>",
			result.TotalCount, result.OnSaleCount, result.UpdatedAt)
	})

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {})

	fmt.Println("\n========================================")
	fmt.Printf("[启动] API 服务已启动: http://localhost%s\n", cfgPort)
	fmt.Println("   GET /api/products  - 全部商品")
	fmt.Println("   GET /api/onsale    - 在售商品")
	fmt.Println("========================================")
	fmt.Printf("[定时] 每 %d 分钟自动爬取更新数据\n", cfgInterval)

	if err := http.ListenAndServe(cfgPort, nil); err != nil {
		log.Fatalf("[错误] 服务器启动失败: %v", err)
	}
}
