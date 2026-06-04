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

	"roco-api/config"
	"roco-api/pkg/model"
	"roco-api/src/notify"
)

// ============================================================
// 缓存
// ============================================================

var (
	Cache      model.CrawlResult
	CacheMutex sync.RWMutex
)

// ============================================================
// 入口
// ============================================================

// Do 执行一次完整爬取
func Do() {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(config.AppConfig.Crawl.TargetURL)
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

	slots := extractSlots(doc)
	products := extractProducts(doc, slots)

	onsaleCount := 0
	for _, p := range products {
		if p.IsOnSale {
			onsaleCount++
		}
	}

	CacheMutex.Lock()
	Cache = model.CrawlResult{
		TimeSlots:   slots,
		Products:    products,
		OnSaleCount: onsaleCount,
		TotalCount:  len(products),
		UpdatedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}
	CacheMutex.Unlock()

	fmt.Printf("[完成] 爬取完成: %d 件商品, %d 件在售中\n", len(products), onsaleCount)

	// 推送
	if config.ServerChanEnabled() && onsaleCount > 0 {
		newOnsale := CollectNew(products)
		if len(newOnsale) > 0 {
			title := fmt.Sprintf("🏪 远行商人更新 · %d 件新上架", len(newOnsale))
			desp := BuildMsg(newOnsale)
			notify.Send(title, desp)
			MarkPushed(newOnsale)
		}
	}
}

// StartCron 启动定时爬取
func StartCron() {
	ticker := time.NewTicker(config.CrawlInterval())
	defer ticker.Stop()
	for range ticker.C {
		fmt.Printf("[%s] [定时] 爬取中...\n", time.Now().Format("15:04:05"))
		Do()
	}
}

// ============================================================
// 提取
// ============================================================

func extractSlots(doc *goquery.Document) []model.ShopSlot {
	var labels []string
	doc.Find(".sp-time-con li").Each(func(i int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if t != "" {
			labels = append(labels, t)
		}
	})
	if len(labels) == 0 {
		doc.Find(".time-con li").Each(func(i int, s *goquery.Selection) {
			t := strings.TrimSpace(s.Text())
			if t != "" {
				labels = append(labels, t)
			}
		})
	}
	if len(labels) == 0 {
		labels = []string{"08:00-12:00", "12:00-16:00", "16:00-20:00", "20:00-24:00"}
	}
	slots := make([]model.ShopSlot, len(labels))
	for i, l := range labels {
		slots[i] = model.ShopSlot{Label: l}
	}
	return slots
}

func extractProducts(doc *goquery.Document, slots []model.ShopSlot) []model.Product {
	now := time.Now().UTC()
	var products []model.Product

	doc.Find(".all_show").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".shop_name").Text())
		if name == "" {
			return
		}

		price := strings.TrimSpace(s.Find(".shop_price").Text())
		price = strings.Replace(price, "价格：", "", 1)

		limit := strings.TrimSpace(s.Find(".gitem em").First().Text())
		imgSrc, _ := s.Find(".gitem img").Attr("src")
		dataTimeStr, _ := s.Attr("data-time")
		onclick, _ := s.Attr("onclick")
		category, desc := parseOnclick(onclick)

		var isOnSale bool
		var remain, slotLabel string

		if dataTimeStr != "" {
			if ts, err := strconv.ParseInt(dataTimeStr, 10, 64); err == nil {
				end := time.Unix(ts, 0)
				start := end.Add(-4 * time.Hour)
				isOnSale = now.After(start) && now.Before(end)
				slotLabel = SlotByTime(dataTimeStr)
				if isOnSale {
					d := end.Sub(now)
					remain = fmt.Sprintf("%02d:%02d:%02d",
						int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
				}
			}
		}

		products = append(products, model.Product{
			Name: name, Price: price, Limit: limit,
			Category: category, Desc: desc,
			ImageURL: imgSrc, IsOnSale: isOnSale,
			RemainStr: remain, SlotLabel: slotLabel,
		})
	})

	return products
}

func parseOnclick(onclick string) (category, desc string) {
	re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
	m := re.FindStringSubmatch(onclick)
	if len(m) >= 5 {
		category = m[3]
		desc = m[4]
	}
	return
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
