package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"roco-api/internal/crawler"
	"roco-api/pkg/model"
)

// Start 启动 HTTP 服务
func Start() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/products", handleProducts)
	http.HandleFunc("/api/onsale", handleOnSale)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	crawler.CacheMutex.RLock()
	result := crawler.Cache
	crawler.CacheMutex.RUnlock()

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

func handleProducts(w http.ResponseWriter, r *http.Request) {
	crawler.CacheMutex.RLock()
	result := crawler.Cache
	crawler.CacheMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(model.APIResponse{
		Code:    200,
		Message: "success",
		Data:    result,
	})
}

func handleOnSale(w http.ResponseWriter, r *http.Request) {
	crawler.CacheMutex.RLock()
	result := crawler.Cache
	crawler.CacheMutex.RUnlock()

	var onsale []model.Product
	for _, p := range result.Products {
		if p.IsOnSale {
			onsale = append(onsale, p)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(model.APIResponse{
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
