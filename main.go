package main

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Product 商品数据结构
type Product struct {
	Name        string // 商品名称
	Price       string // 价格
	Limit       string // 限购数量
	Category    string // 商品分类
	Desc        string // 商品描述
	ImageURL    string // 图片链接
	DataTime    int64  // 时间戳（Unix秒）
	IsOnSale    bool   // 是否在售中
	RemainTime  string // 剩余时间文本
}

func main() {
	url := "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0"

	fmt.Println("🔄 正在爬取远行商人数据...")
	fmt.Printf("📡 URL: %s\n\n", url)

	// ============================================================
	// 1. 发送 HTTP 请求获取页面
	// ============================================================
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("请求异常，状态码: %d", resp.StatusCode)
	}

	// ============================================================
	// 2. 解析 HTML
	// ============================================================
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatalf("解析 HTML 失败: %v", err)
	}

	// 获取页面中的时间槽信息（08:00-12:00 等）
	timeSlots := extractTimeSlots(doc)
	fmt.Println("📅 今日时段:", timeSlots)
	fmt.Println()

	// ============================================================
	// 3. 提取商品数据
	// ============================================================
	var products []Product

	// 使用 UTC 时间比较（data-time 是 UTC 时间戳）
	now := time.Now().UTC()

	doc.Find(".all_show").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Find(".shop_name").Text())
		if name == "" {
			return // 跳过空项
		}

		// 价格
		price := strings.TrimSpace(s.Find(".shop_price").Text())
		price = strings.Replace(price, "价格：", "", 1)
		price = strings.TrimSpace(price)

		// 限购
		limit := strings.TrimSpace(s.Find(".gitem em").First().Text())

		// 图片链接
		imgSrc, _ := s.Find(".gitem img").Attr("src")

		// data-time 时间戳（Unix秒）- 直接在 li 上
		dataTimeStr, _ := s.Attr("data-time")

		// 从 onclick 中解析分类和描述
		onclick, _ := s.Attr("onclick")
		category, desc := parseOnclick(onclick)

		// 判断是否在售中
		// data-time = 时段结束时间 (Unix 秒, UTC)
		// 当前时间 < 结束时间 => 在售中
		var endTime time.Time
		var isOnSale bool
		if dataTimeStr != "" {
			if ts, err := strconv.ParseInt(dataTimeStr, 10, 64); err == nil {
				endTime = time.Unix(ts, 0)              // UTC
				startTime := endTime.Add(-4 * time.Hour) // 每个时段4小时
				isOnSale = now.After(startTime) && now.Before(endTime)
			}
		}

		// 计算剩余时间
		remainTime := ""
		if isOnSale {
			diff := endTime.Sub(now)
			h := int(diff.Hours())
			m := int(diff.Minutes()) % 60
			s := int(diff.Seconds()) % 60
			remainTime = fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		}

		products = append(products, Product{
			Name:       name,
			Price:      price,
			Limit:      limit,
			Category:   category,
			Desc:       desc,
			ImageURL:   imgSrc,
			DataTime:   endTime.Unix(),
			IsOnSale:   isOnSale,
			RemainTime: remainTime,
		})
	})

	// ============================================================
	// 4. 输出结果
	// ============================================================
	if len(products) == 0 {
		fmt.Println("❌ 未找到商品数据")
		return
	}

	fmt.Println("========== 🏪 远行商人商品列表 ==========\n")

	onsaleCount := 0
	for i, p := range products {
		status := ""
		if p.IsOnSale {
			status = fmt.Sprintf("✅ 在售中 (剩余 %s)", p.RemainTime)
			onsaleCount++
		} else {
			status = "❌ 已结束"
		}

		fmt.Printf("【%d】%s\n", i+1, p.Name)
		fmt.Printf("    💰 价格: %s\n", p.Price)
		fmt.Printf("    📦 限购: %s\n", p.Limit)
		fmt.Printf("    📂 分类: %s\n", p.Category)
		fmt.Printf("    📊 状态: %s\n", status)
		fmt.Println()
	}

	fmt.Printf("📈 共 %d 件商品，其中 %d 件在售中\n", len(products), onsaleCount)
	fmt.Println("==========================================")
}

// extractTimeSlots 提取页面中的时间段信息
func extractTimeSlots(doc *goquery.Document) []string {
	var slots []string
	doc.Find(".sp-time-con li").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			slots = append(slots, text)
		}
	})
	if len(slots) == 0 {
		// 备用：从其他选择器获取
		doc.Find(".time-con li").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				slots = append(slots, text)
			}
		})
	}
	return slots
}

// parseOnclick 从 onclick 属性中解析商品分类和描述
func parseOnclick(onclick string) (category, desc string) {
	// onclick 格式: showShopinfo('图片URL','商品名','分类','描述')
	re := regexp.MustCompile(`'([^']*)','([^']*)','([^']*)','([^']*)'`)
	matches := re.FindStringSubmatch(onclick)
	if len(matches) >= 5 {
		category = matches[3]
		desc = matches[4]
	}
	return
}
