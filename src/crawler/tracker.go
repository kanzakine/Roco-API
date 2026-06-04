package crawler

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"roco-api/pkg/model"
)

// ============================================================
// 推送追踪
// ============================================================

// pushData 维护已推送和过往商品记录
type pushData struct {
	notified map[string]string    // 已推送商品名 → 时段（跨日重置）
	past     []model.NotifyRecord // 今日过往商品（已推送过且已下架）
	today    string               // 日期 YYYY-MM-DD
	mu       sync.Mutex
}

var tracker = &pushData{}

func InitTracker() {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.notified = make(map[string]string)
	tracker.past = nil
	tracker.today = time.Now().Format("2006-01-02")
}

func (t *pushData) ensureToday() {
	if t.today != time.Now().Format("2006-01-02") {
		t.notified = make(map[string]string)
		t.past = nil
		t.today = time.Now().Format("2006-01-02")
	}
}

// CollectNew 收集未推送过的在售商品，同时从当前页面提取所有已结束商品作为"过往"。
func CollectNew(products []model.Product) []model.Product {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.ensureToday()

	// 第 1 步：收集新上架在售商品
	var fresh []model.Product
	for _, p := range products {
		if p.IsOnSale {
			if _, ok := tracker.notified[p.Name]; !ok {
				fresh = append(fresh, p)
			}
		}
	}

	// 第 2 步：从页面所有商品中提取已结束的，加入过往（去重）
	//   - 不在售（IsOnSale==false）且时段不为空（有 SlotLabel）
	//   - 不在 past 中 → 追加
	pastMap := make(map[string]bool, len(tracker.past))
	for _, r := range tracker.past {
		pastMap[r.Name] = true
	}
	for _, p := range products {
		if p.IsOnSale || p.SlotLabel == "" {
			continue
		}
		if !pastMap[p.Name] {
			tracker.past = append(tracker.past, model.NotifyRecord{
				Name: p.Name, Slot: p.SlotLabel, Price: p.Price,
			})
			pastMap[p.Name] = true
		}
	}

	return fresh
}

func MarkPushed(products []model.Product) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.notified == nil {
		tracker.notified = map[string]string{}
	}
	for _, p := range products {
		tracker.notified[p.Name] = p.SlotLabel
	}
}

func PastList() []model.NotifyRecord {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	r := make([]model.NotifyRecord, len(tracker.past))
	copy(r, tracker.past)
	return r
}

// IsFirst 今日是否尚未推送过任何商品
func IsFirst() bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return len(tracker.notified) == 0
}

// SlotByTime 从 data-time 时间戳推断时段标签
func SlotByTime(dt string) string {
	ts, _ := parseInt64(dt)
	if ts == 0 {
		return ""
	}
	h := time.Unix(ts, 0).In(time.FixedZone("CST", 8*3600)).Hour()
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

func CurSlot() string {
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

// slotOrder 返回时段的排序键（首小时数值），用于排序
func slotOrder(label string) int {
	h := 0
	fmt.Sscanf(label, "%d", &h)
	return h
}

func BuildMsg(onsale []model.Product) string {
	past := PastList()
	// 过往商品按时段从早到晚排序
	sort.Slice(past, func(i, j int) bool {
		return slotOrder(past[i].Slot) < slotOrder(past[j].Slot)
	})
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 🕐 %s\n\n", CurSlot()))
	for _, p := range onsale {
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
	// 过往商品：只要有数据就显示
	if len(past) > 0 {
		b.WriteString("---\n### 📜 今日过往商品\n\n")
		for _, r := range past {
			b.WriteString(fmt.Sprintf("- %s（%s %s）\n", r.Name, r.Slot, r.Price))
		}
	}
	b.WriteString("---\n📡 [查看完整页面](http://localhost:8008)\n")
	return b.String()
}
