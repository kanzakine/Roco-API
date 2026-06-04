package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ============================================================
// 配置项 - 改成你自己的信息
// ============================================================
const (
	targetURL  = "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0"
	crawlInterval = 3 * time.Minute // 每3分钟爬取一次
	serverPort    = ":8008"         // API 服务端口
)

// ============================================================
// 数据结构
// ============================================================

// ShopSlot 时段信息
type ShopSlot struct {
	Label string `json:"label"` // 如 "08:00-12:00"
}

// Product 商品
type Product struct {
	Name      string `json:"name"`       // 商品名称
	Price     string `json:"price"`      // 价格
	Limit     string `json:"limit"`      // 限购
	Category  string `json:"category"`   // 分类
	Desc      string `json:"desc"`       // 描述
	ImageURL  string `json:"image_url"`  // 图片
	IsOnSale  bool   `json:"is_on_sale"` // 是否在售
	RemainStr string `json:"remain"`     // 剩余时间文本
}

// CrawlResult 爬取结果（整个缓存）
type CrawlResult struct {
	TimeSlots   []ShopSlot `json:"time_slots"`   // 四个时段
	Products    []Product  `json:"products"`      // 所有商品
	OnSaleCount int        `json:"on_sale_count"` // 在售数量
	TotalCount  int        `json:"total_count"`   // 总商品数
	UpdatedAt   string     `json:"updated_at"`    // 最后更新时间
}

// APIResponse API 通用响应格式
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ============================================================
// 全局缓存（带读写锁，线程安全）
// ============================================================
var (
	cache      CrawlResult
	cacheMutex sync.RWMutex
)

// ============================================================
// 主函数
// ============================================================
func main() {
	// 1. 启动时立即爬取一次
	fmt.Println("🔄 首次爬取远行商人数据...")
	doCrawl()

	// 2. 后台定时爬取
	go startCronCrawl()

	// 3. 启动 HTTP API 服务
	startHTTPServer()
}

// ============================================================
// 定时爬取
// ============================================================
func startCronCrawl() {
	ticker := time.NewTicker(crawlInterval)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Printf("[%s] ⏰ 定时爬取中...\n", time.Now().Format("15:04:05"))
		doCrawl()
	}
}

// ============================================================
// 爬取逻辑
// ============================================================
func doCrawl() {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(targetURL)
	if err != nil {
		log.Printf("❌ 请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ 请求异常，状态码: %d", resp.StatusCode)
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("❌ 解析 HTML 失败: %v", err)
		return
	}

	// 提取时段
	timeSlots := extractTimeSlots(doc)
	var slots []ShopSlot
	for _, s := range timeSlots {
		slots = append(slots, ShopSlot{Label: s})
	}
	// 如果没解析到，用默认的四个时段
	if len(slots) == 0 {
		slots = []ShopSlot{
			{Label: "08:00-12:00"},
			{Label: "12:00-16:00"},
			{Label: "16:00-20:00"},
			{Label: "20:00-24:00"},
		}
	}

	// 提取商品
	now := time.Now().UTC()
	var products []Product

	doc.Find(".all_show").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".shop_name").Text())
		if name == "" {
			return
		}

		price := strings.TrimSpace(s.Find(".shop_price").Text())
		price = strings.Replace(price, "价格：", "", 1)
		price = strings.TrimSpace(price)

		limit := strings.TrimSpace(s.Find(".gitem em").First().Text())
		imgSrc, _ := s.Find(".gitem img").Attr("src")
		dataTimeStr, _ := s.Attr("data-time")

		onclick, _ := s.Attr("onclick")
		category, desc := parseOnclick(onclick)

		var isOnSale bool
		var remainStr string
		if dataTimeStr != "" {
			if ts, err := strconv.ParseInt(dataTimeStr, 10, 64); err == nil {
				endTime := time.Unix(ts, 0)
				startTime := endTime.Add(-4 * time.Hour)
				isOnSale = now.After(startTime) && now.Before(endTime)
				if isOnSale {
					diff := endTime.Sub(now)
					h := int(diff.Hours())
					m := int(diff.Minutes()) % 60
					s := int(diff.Seconds()) % 60
					remainStr = fmt.Sprintf("%02d:%02d:%02d", h, m, s)
				}
			}
		}

		products = append(products, Product{
			Name:      name,
			Price:     price,
			Limit:     limit,
			Category:  category,
			Desc:      desc,
			ImageURL:  imgSrc,
			IsOnSale:  isOnSale,
			RemainStr: remainStr,
		})
	})

	// 统计
	onsaleCount := 0
	for _, p := range products {
		if p.IsOnSale {
			onsaleCount++
		}
	}

	// 更新缓存
	cacheMutex.Lock()
	cache = CrawlResult{
		TimeSlots:   slots,
		Products:    products,
		OnSaleCount: onsaleCount,
		TotalCount:  len(products),
		UpdatedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}
	cacheMutex.Unlock()

	fmt.Printf("✅ 爬取完成: %d 件商品, %d 件在售中\n", len(products), onsaleCount)
}

// ============================================================
// HTTP API 服务
// ============================================================
func startHTTPServer() {
	// 路由注册
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/products", handleProducts)
	http.HandleFunc("/api/onsale", handleOnSale)

	fmt.Println("\n========================================")
	fmt.Printf("🚀 API 服务已启动: http://localhost%s\n", serverPort)
	fmt.Println("   可用接口:")
	fmt.Println("     GET  /              - 服务状态页")
	fmt.Println("     GET  /api/products  - 所有商品数据 (JSON)")
	fmt.Println("     GET  /api/onsale    - 仅在售商品 (JSON)")
	fmt.Println("========================================")
	fmt.Println("⏰ 每 3 分钟自动爬取更新数据")

	err := http.ListenAndServe(serverPort, nil)
	if err != nil {
		log.Fatalf("❌ 服务器启动失败: %v", err)
	}
}

// handleHome 首页 - 显示状态
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cacheMutex.RLock()
	result := cache
	cacheMutex.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><title>远行商人 API</title>
<style>
  body { font-family: Arial, sans-serif; max-width: 800px; margin: 40px auto; padding: 0 20px; }
  h1 { color: #333; }
  .card { background: #f5f5f5; border-radius: 8px; padding: 16px; margin: 12px 0; }
  .onsale { border-left: 4px solid #4caf50; }
  .ended { border-left: 4px solid #f44336; }
  .tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 13px; }
  .tag-ok { background: #4caf50; color: #fff; }
  .tag-end { background: #f44336; color: #fff; }
  .slot { display: inline-block; background: #e3f2fd; padding: 4px 12px; border-radius: 4px; margin: 4px; }
  code { background: #e8e8e8; padding: 2px 6px; border-radius: 3px; }
  a { color: #1976d2; }
</style></head>
<body>
  <h1>🏪 洛克王国 · 远行商人 API</h1>
  <div class="card">
    <p>📅 更新时间: <strong>%s</strong></p>
    <p>📦 商品总数: <strong>%d</strong> &nbsp;|&nbsp; ✅ 在售: <strong>%d</strong></p>
    <p>🕐 时段:
`, result.UpdatedAt, result.TotalCount, result.OnSaleCount)

	for _, slot := range result.TimeSlots {
		fmt.Fprintf(w, `      <span class="slot">%s</span>
`, slot.Label)
	}

	fmt.Fprintf(w, `    </p>
  </div>
  <h2>API 接口</h2>
  <div class="card">
    <p><strong>GET <code><a href="/api/products">/api/products</a></code></strong> — 全部商品数据 (JSON)</p>
    <p><strong>GET <code><a href="/api/onsale">/api/onsale</a></code></strong> — 仅在售商品 (JSON)</p>
  </div>
  <h2>当前商品</h2>
`)
	for _, p := range result.Products {
		cls := "ended"
		tagCls := "tag-end"
		statusText := "❌ 已结束"
		if p.IsOnSale {
			cls = "onsale"
			tagCls = "tag-ok"
			statusText = fmt.Sprintf("✅ 在售 · 剩余 %s", p.RemainStr)
		}
		fmt.Fprintf(w, `  <div class="card %s">
    <strong>%s</strong> <span class="tag %s">%s</span>
    <p>💰 %s &nbsp; 📦 %s &nbsp; 📂 %s</p>
  </div>
`, cls, p.Name, tagCls, statusText, p.Price, p.Limit, p.Category)
	}

	fmt.Fprintf(w, `</body></html>`)
}

// handleProducts 返回所有商品 JSON
func handleProducts(w http.ResponseWriter, r *http.Request) {
	cacheMutex.RLock()
	result := cache
	cacheMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(APIResponse{
		Code:    200,
		Message: "success",
		Data:    result,
	})
}

// handleOnSale 返回仅在售商品 JSON
func handleOnSale(w http.ResponseWriter, r *http.Request) {
	cacheMutex.RLock()
	result := cache
	cacheMutex.RUnlock()

	var onsale []Product
	for _, p := range result.Products {
		if p.IsOnSale {
			onsale = append(onsale, p)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(APIResponse{
		Code:    200,
		Message: "success",
		Data: map[string]interface{}{
			"time_slots":    result.TimeSlots,
			"products":      onsale,
			"on_sale_count": len(onsale),
			"updated_at":    result.UpdatedAt,
		},
	})
}

// ============================================================
// 辅助函数
// ============================================================

// extractTimeSlots 提取时段
func extractTimeSlots(doc *goquery.Document) []string {
	var slots []string
	doc.Find(".sp-time-con li").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			slots = append(slots, text)
		}
	})
	if len(slots) == 0 {
		doc.Find(".time-con li").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				slots = append(slots, text)
			}
		})
	}
	return slots
}

// parseOnclick 从 onclick 属性解析商品分类和描述
func parseOnclick(onclick string) (category, desc string) {
	re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
	matches := re.FindStringSubmatch(onclick)
	if len(matches) >= 5 {
		category = matches[3]
		desc = matches[4]
	}
	return
}
