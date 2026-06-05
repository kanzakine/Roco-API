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
// 数据结构
// ============================================================

// ShopSlot 时段信息
type ShopSlot struct {
	Label string `json:"label"` // 如 "08:00-12:00"
}

// Product 商品
type Product struct {
	Name       string `json:"name"`        // 商品名称
	Price      string `json:"price"`       // 价格
	Limit      string `json:"limit"`       // 限购
	Category   string `json:"category"`    // 分类
	Desc       string `json:"desc"`        // 描述
	ImageURL   string `json:"image_url"`   // 图片
	IsOnSale   bool   `json:"is_on_sale"`  // 是否在售（已开始且未结束）
	HasEnded   bool   `json:"has_ended"`   // 已结束
	IsUpcoming bool   `json:"is_upcoming"` // 即将开售（还没到时间）
	RemainStr  string `json:"remain"`      // 倒计时 HH:MM:SS（在售中时有效）
	SlotLabel  string `json:"slot_label"`  // 所属时段，如 "08:00-12:00"
}

// CrawlResult 爬取结果（整个缓存）
type CrawlResult struct {
	TimeSlots   []ShopSlot `json:"time_slots"`    // 四个时段
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
// 推送追踪 — 控制推送时机和次数
// ============================================================

// pushRecord 保存已推送过的商品标识
type pushRecord struct {
	Date      string            // 日期 YYYY-MM-DD
	SentNames map[string]bool   // 已推送过的商品名
	PastNames map[string]string // 过往商品名→时段（今日已结束的）
}

var (
	pushTracker     pushRecord
	pushTrackerLock sync.Mutex
)

// initPushTracker 初始化或重置推送追踪
func initPushTracker() {
	today := time.Now().Format("2006-01-02")
	pushTracker = pushRecord{
		Date:      today,
		SentNames: make(map[string]bool),
		PastNames: make(map[string]string),
	}
}

// needPush 判断是否需要推送：有新上架商品时推送
func needPush(onsaleProducts []Product) bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	// 如果日期变了，重置追踪
	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		initPushTracker()
	}

	// 检查是否有新商品未推送过
	for _, p := range onsaleProducts {
		if !pushTracker.SentNames[p.Name] {
			return true
		}
	}
	return false
}

// markPushed 标记商品已推送
func markPushed(onsaleProducts []Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	for _, p := range onsaleProducts {
		pushTracker.SentNames[p.Name] = true
	}
}

// updatePastProducts 更新今日过往（已结束）商品
func updatePastProducts(products []Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		initPushTracker()
	}

	for _, p := range products {
		// 只有确实已结束（结束时间已过）的商品才记入过往
		if p.HasEnded && p.SlotLabel != "" {
			pushTracker.PastNames[p.Name] = p.SlotLabel
		}
	}
}

// getPastProducts 获取今日过往商品（简短格式）
func getPastProducts() []string {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	if pushTracker.Date != time.Now().Format("2006-01-02") {
		return nil
	}

	var result []string
	for name, slot := range pushTracker.PastNames {
		result = append(result, fmt.Sprintf("- %s（%s）", name, slot))
	}
	return result
}

// isFirstBatch 判断是否是今日首批推送（过往商品为空则为首批）
func isFirstBatch() bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()
	return len(pushTracker.PastNames) == 0
}

// ============================================================
// 主函数
// ============================================================
func main() {
	// 0. 加载配置文件
	if err := LoadConfig("config.json"); err != nil {
		log.Fatalf("[错误] %v", err)
	}

	// 如果配置了 Server酱，提示已启用
	if ServerChanEnabled() {
		fmt.Println("[通知] Server酱 推送已启用")
	} else {
		fmt.Println("[信息] Server酱 推送未配置（如需启用，请填写 config.json 中的 uid 和 sendkey）")
	}

	// 1. 初始化推送追踪
	initPushTracker()

	// 2. 启动时立即爬取一次
	fmt.Println("[任务] 首次爬取远行商人数据...")
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
	ticker := time.NewTicker(CrawlInterval())
	defer ticker.Stop()

	for range ticker.C {
		fmt.Printf("[定时] 爬取中...\n")
		doCrawl()
	}
}

// ============================================================
// 爬取逻辑
// ============================================================
func doCrawl() {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(appConfig.Crawl.TargetURL)
	if err != nil {
		log.Printf("[错误] 请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[错误] 请求异常，状态码: %d", resp.StatusCode)
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Printf("[错误] 解析 HTML 失败: %v", err)
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

	// 提取商品并关联时段
	now := time.Now().UTC()
	var products []Product

	// 先确定当前在哪一时段
	currentSlotLabel := currentSlot(slots, now)

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

		var hasEnded bool
		var remainStr string
		var slotLabel string

		if dataTimeStr != "" {
			if ts, err := strconv.ParseInt(dataTimeStr, 10, 64); err == nil {
				// data-time 是时间槽结束时间（UTC Unix 时间戳）
				endTime := time.Unix(ts, 0)
				hasEnded = now.After(endTime)
				// 计算商品所属时段（从结束时间反推开始时间）
				startTime := endTime.Add(-4 * time.Hour)
				slotLabel = slotForTime(slots, startTime, endTime)

				// 只要没结束就显示倒计时
				if !hasEnded {
					diff := endTime.Sub(now)
					h := int(diff.Hours())
					m := int(diff.Minutes()) % 60
					s := int(diff.Seconds()) % 60
					remainStr = fmt.Sprintf("%02d:%02d:%02d", h, m, s)
				}
			}
		}

		products = append(products, Product{
			Name:       name,
			Price:      price,
			Limit:      limit,
			Category:   category,
			Desc:       desc,
			ImageURL:   imgSrc,
			IsOnSale:   !hasEnded, // 没结束就是在售
			HasEnded:   hasEnded,
			IsUpcoming: false, // 简化：全部归为在售
			RemainStr:  remainStr,
			SlotLabel:  slotLabel,
		})
	})

	// 统计
	onsaleCount := 0
	for _, p := range products {
		if p.IsOnSale {
			onsaleCount++
		}
	}

	// 更新过往商品（已结束的）
	updatePastProducts(products)

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

	fmt.Printf("[完成] 爬取完成: %d 件商品, %d 件在售中\n", len(products), onsaleCount)

	// 推送逻辑：只在有新上架商品时推送
	if ServerChanEnabled() && onsaleCount > 0 {
		var onsaleProducts []Product
		for _, p := range products {
			if p.IsOnSale {
				onsaleProducts = append(onsaleProducts, p)
			}
		}

		if needPush(onsaleProducts) {
			title := fmt.Sprintf("🏪 远行商人更新 · %d 件在售", onsaleCount)
			desp := buildPushMessage(onsaleProducts, currentSlotLabel)
			SendServerChan(title, desp)
			markPushed(onsaleProducts)
		}
	}
}

// ============================================================
// HTTP API 服务
// ============================================================
func startHTTPServer() {
	// 路由注册
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/products", handleProducts)
	http.HandleFunc("/api/onsale", handleOnSale)

	port := appConfig.Server.Port
	fmt.Println("\n========================================")
	fmt.Printf("[启动] API 服务已启动: http://localhost%s\n", port)
	fmt.Println("   GET /api/products  - 全部商品")
	fmt.Println("   GET /api/onsale    - 在售商品")
	fmt.Println("========================================")
	fmt.Printf("[定时] 每 %d 分钟自动爬取更新数据\n", appConfig.Crawl.Interval)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatalf("[错误] 服务器启动失败: %v", err)
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

	// 构建所有商品卡片HTML（一次性写入，避免fmt.Sprintf参数数量问题）
	var buf strings.Builder
	buf.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><title>远行商人</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial,sans-serif;max-width:800px;margin:40px auto;padding:0 20px}
h1{color:#333;margin-bottom:12px}
.card{background:#f5f5f5;border-radius:8px;padding:16px;margin:12px 0}
.onsale{border-left:4px solid #4caf50}
.ended{border-left:4px solid #f44336}
.upcoming{border-left:4px solid #ff9800}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:13px}
.tag-ok{background:#4caf50;color:#fff}
.tag-end{background:#f44336;color:#fff}
.slot{display:inline-block;background:#e3f2fd;padding:4px 12px;border-radius:4px;margin:4px}
.cd{font-weight:bold}
</style></head>
<body>
<h1>🏪 洛克王国 · 远行商人</h1>
<p>`)
	buf.WriteString(fmt.Sprintf("📅 更新时间: <strong>%s</strong> &nbsp; 📦 %d件 ✅ %d件在售</p>\n<p>🕐 时段:", result.UpdatedAt, result.TotalCount, result.OnSaleCount))
	for _, slot := range result.TimeSlots {
		buf.WriteString(fmt.Sprintf(`<span class="slot">%s</span>`, slot.Label))
	}
	buf.WriteString("</p>\n<h2 style=\"margin-top:20px\">当前商品</h2>\n")

	for _, p := range result.Products {
		var cls, tagCls, badge string
		if p.IsOnSale {
			cls = "onsale"
			tagCls = "tag-ok"
			badge = "✅ 在售"
		} else {
			cls = "ended"
			tagCls = "tag-end"
			badge = "❌ 已结束"
		}
		slotInfo := fmt.Sprintf(`<span class="slot">🕐 %s</span>`, p.SlotLabel)

		if p.IsOnSale {
			buf.WriteString(fmt.Sprintf(`<div class="card %s" data-remain="%s">
  <strong>%s</strong> <span class="tag %s">%s</span>
  <p>💰 %s &nbsp; 📦 %s &nbsp; 📂 %s %s</p>
  <p style="color:#4caf50">⏱ 剩余 <span class="cd">%s</span></p>
</div>`, cls, p.RemainStr, p.Name, tagCls, badge, p.Price, p.Limit, p.Category, slotInfo, p.RemainStr))
		} else {
			buf.WriteString(fmt.Sprintf(`<div class="card %s">
  <strong>%s</strong> <span class="tag %s">%s</span>
  <p>💰 %s &nbsp; 📦 %s &nbsp; 📂 %s %s</p>
  <p style="color:#999">—</p>
</div>`, cls, p.Name, tagCls, badge, p.Price, p.Limit, p.Category, slotInfo))
		}
	}

	buf.WriteString(`<script>
(function(){
setInterval(function(){
  document.querySelectorAll('[data-remain]').forEach(function(e){
    var r=e.getAttribute('data-remain');
    if(!r||r.indexOf(':')<0)return;
    var a=r.split(':');
    var s=parseInt(a[0],10)*3600+parseInt(a[1],10)*60+parseInt(a[2],10);
    if(s<=0)return;
    s--;
    function P(n){return n<10?'0'+n:''+n}
    var v=P(Math.floor(s/3600))+':'+P(Math.floor((s%3600)/60))+':'+P(s%60);
    e.setAttribute('data-remain',v);
    var c=e.querySelector('.cd');
    if(c)c.textContent=v;
  });
},1000);
})();
</script></body></html>`)

	w.Write([]byte(buf.String()))
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

// buildPushMessage 构造推送内容
// 格式：时段 → 在售商品详情 → 今日过往商品（非首批时显示）
func buildPushMessage(onsaleProducts []Product, currentSlotLabel string) string {
	var b strings.Builder

	// 时段标题
	b.WriteString(fmt.Sprintf("## 🕐 %s\n\n", currentSlotLabel))

	// 在售商品
	for _, p := range onsaleProducts {
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

	// 今日过往商品（只在有记录时显示）
	if !isFirstBatch() {
		pastList := getPastProducts()
		if len(pastList) > 0 {
			b.WriteString("---\n")
			b.WriteString("### 📜 今日过往商品\n\n")
			for _, item := range pastList {
				b.WriteString(item + "\n")
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("---\n")
	b.WriteString("📡 [查看完整页面](http://localhost" + appConfig.Server.Port + ")\n")
	return b.String()
}

// currentSlot 根据当前时间判断所在时段
func currentSlot(slots []ShopSlot, now time.Time) string {
	if len(slots) == 0 {
		return "未知时段"
	}
	currentHour := now.In(time.FixedZone("CST", 8*3600)).Hour()
	for _, slot := range slots {
		startH, endH := parseSlotHours(slot.Label)
		if currentHour >= startH && currentHour < endH {
			return slot.Label
		}
	}
	// 默认返回最后一个时段
	return slots[len(slots)-1].Label
}

// slotForTime 根据商品起止时间判断其所属时段
func slotForTime(slots []ShopSlot, startTime, endTime time.Time) string {
	if len(slots) == 0 {
		return "未知时段"
	}
	startH := startTime.In(time.FixedZone("CST", 8*3600)).Hour()
	for _, slot := range slots {
		sh, eh := parseSlotHours(slot.Label)
		if startH >= sh && startH < eh {
			return slot.Label
		}
	}
	return slots[len(slots)-1].Label
}

// parseSlotHours 解析时段标签，如 "08:00-12:00" → (8, 12)
func parseSlotHours(label string) (int, int) {
	parts := strings.Split(label, "-")
	if len(parts) != 2 {
		return 0, 24
	}
	startH, _ := strconv.Atoi(strings.Split(parts[0], ":")[0])
	endH, _ := strconv.Atoi(strings.Split(parts[1], ":")[0])
	return startH, endH
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
