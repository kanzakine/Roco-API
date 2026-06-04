package crawler

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"roco-api/internal/config"
	"roco-api/internal/notify"
	"roco-api/pkg/model"
)

// ============================================================
// 全局缓存
// ============================================================

var (
	Cache      model.CrawlResult
	CacheMutex sync.RWMutex
)

// ============================================================
// 推送追踪
// ============================================================

type pushRecord struct {
	Date      string
	SentNames map[string]bool
	PastNames map[string]string
}

var (
	pushTracker     pushRecord
	pushTrackerLock sync.Mutex
)

// InitTracker 初始化推送追踪
func InitTracker() {
	today := time.Now().Format("2006-01-02")
	pushTracker = pushRecord{
		Date:      today,
		SentNames: make(map[string]bool),
		PastNames: make(map[string]string),
	}
}

func needPush(onsaleProducts []model.Product) bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		InitTracker()
	}

	for _, p := range onsaleProducts {
		if !pushTracker.SentNames[p.Name] {
			return true
		}
	}
	return false
}

func markPushed(onsaleProducts []model.Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	for _, p := range onsaleProducts {
		pushTracker.SentNames[p.Name] = true
	}
}

func updatePastProducts(products []model.Product) {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()

	today := time.Now().Format("2006-01-02")
	if pushTracker.Date != today {
		InitTracker()
	}

	for _, p := range products {
		if !p.IsOnSale && p.SlotLabel != "" {
			pushTracker.PastNames[p.Name] = p.SlotLabel
		}
	}
}

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

func isFirstBatch() bool {
	pushTrackerLock.Lock()
	defer pushTrackerLock.Unlock()
	return len(pushTracker.PastNames) == 0
}

// ============================================================
// 爬取逻辑
// ============================================================

// Do 执行一次完整的爬取流程
func Do() {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(config.AppConfig.Crawl.TargetURL)
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
	slots := buildSlots(timeSlots)

	// 提取商品并关联时段
	now := time.Now().UTC()
	products := extractProducts(doc, slots, now)

	// 统计
	onsaleCount := 0
	for _, p := range products {
		if p.IsOnSale {
			onsaleCount++
		}
	}

	// 更新缓存
	CacheMutex.Lock()
	Cache = model.CrawlResult{
		TimeSlots:   slots,
		Products:    products,
		OnSaleCount: onsaleCount,
		TotalCount:  len(products),
		UpdatedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}
	CacheMutex.Unlock()

	fmt.Printf("✅ 爬取完成: %d 件商品, %d 件在售中\n", len(products), onsaleCount)

	// 推送逻辑
	if config.ServerChanEnabled() && onsaleCount > 0 {
		newItems := CollectNew(products)
		if len(newItems) > 0 {
			title := fmt.Sprintf("🏪 新商品上架 · %d 件在售", onsaleCount)
			desp := BuildMsg(newItems)
			notify.Send(title, desp)
			MarkPushed(newItems)
		}
	}
}

// ============================================================
// 内部辅助
// ============================================================

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

func buildSlots(rawSlots []string) []model.ShopSlot {
	var slots []model.ShopSlot
	for _, s := range rawSlots {
		slots = append(slots, model.ShopSlot{Label: s})
	}
	if len(slots) == 0 {
		slots = []model.ShopSlot{
			{Label: "08:00-12:00"},
			{Label: "12:00-16:00"},
			{Label: "16:00-20:00"},
			{Label: "20:00-24:00"},
		}
	}
	return slots
}

func extractProducts(doc *goquery.Document, slots []model.ShopSlot, now time.Time) []model.Product {
	var products []model.Product

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
		var slotLabel string

		// data-time 是商品时段结束时间的 UTC 时间戳（秒）
		// 每个时段持续 4 小时
		if dataTimeStr != "" {
			if ts, err := strconv.ParseInt(dataTimeStr, 10, 64); err == nil {
				endTime := time.Unix(ts, 0)              // 结束时间（UTC）
				startTime := endTime.Add(-4 * time.Hour) // 开始时间 = 结束-4h

				// 当前时间在 [startTime, endTime) 范围内 => 在售
				isOnSale = now.After(startTime) && now.Before(endTime)

				// 计算所属时段标签
				slotLabel = SlotByTime(dataTimeStr)

				if isOnSale {
					diff := endTime.Sub(now)
					h := int(diff.Hours())
					m := int(diff.Minutes()) % 60
					s := int(diff.Seconds()) % 60
					remainStr = fmt.Sprintf("%02d:%02d:%02d", h, m, s)
				} else if now.After(endTime) {
					remainStr = "已结束"
				} else {
					remainStr = "待上架"
				}
			}
		}

		products = append(products, model.Product{
			Name:      name,
			Price:     price,
			Limit:     limit,
			Category:  category,
			Desc:      desc,
			ImageURL:  imgSrc,
			IsOnSale:  isOnSale,
			RemainStr: remainStr,
			SlotLabel: slotLabel,
		})
	})

	return products
}

func parseOnclick(onclick string) (category, desc string) {
	re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
	matches := re.FindStringSubmatch(onclick)
	if len(matches) >= 5 {
		category = matches[3]
		desc = matches[4]
	}
	return
}

// ============================================================
// 时段工具
// ============================================================

func currentSlot(slots []model.ShopSlot, now time.Time) string {
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
	return slots[len(slots)-1].Label
}

// ============================================================
// 定时爬取（实现在 tracker.go 中）
// ============================================================
