package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"roco-api/pkg/model"
	"roco-api/src/crawler"
)

// Start 注册 HTTP 路由
func Start() {
	http.HandleFunc("/", home)
	http.HandleFunc("/api/products", products)
	http.HandleFunc("/api/onsale", onsale)
}

func home(w http.ResponseWriter, r *http.Request) {
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
  body{font-family:Arial,sans-serif;max-width:800px;margin:40px auto;padding:0 20px}
  h1{color:#333}.card{background:#f5f5f5;border-radius:8px;padding:16px;margin:12px 0}
  .onsale{border-left:4px solid #4caf50}.ended{border-left:4px solid #f44336}
  .tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:13px}
  .tag-ok{background:#4caf50;color:#fff}.tag-end{background:#f44336;color:#fff}
  .slot{display:inline-block;background:#e3f2fd;padding:4px 12px;border-radius:4px;margin:4px}
</style></head>
<body>
  <h1>🏪 洛克王国 · 远行商人 API</h1>
  <div class="card">
    <p>📅 更新时间: <strong>%s</strong></p>
    <p>📦 商品总数: <strong>%d</strong> | ✅ 在售: <strong>%d</strong></p>
    <p>🕐 时段:
`, result.UpdatedAt, result.TotalCount, result.OnSaleCount)

	for _, slot := range result.TimeSlots {
		fmt.Fprintf(w, `<span class="slot">%s</span> `, slot.Label)
	}

	fmt.Fprintf(w, `</p></div><h2>当前商品</h2>`)
	for _, p := range result.Products {
		cls, tagCls, status := "ended", "tag-end", "❌ 已结束"
		if p.IsOnSale {
			cls, tagCls = "onsale", "tag-ok"
			status = fmt.Sprintf("✅ 在售 · 剩余 %s", p.RemainStr)
		}
		fmt.Fprintf(w, `<div class="card %s"><strong>%s</strong> <span class="tag %s">%s</span><p>💰 %s | 📦 %s | 📂 %s</p></div>`, cls, p.Name, tagCls, status, p.Price, p.Limit, p.Category)
	}

	fmt.Fprintf(w, `</body></html>`)
}

func products(w http.ResponseWriter, r *http.Request) {
	crawler.CacheMutex.RLock()
	result := crawler.Cache
	crawler.CacheMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(model.APIResponse{Code: 200, Message: "success", Data: result})
}

func onsale(w http.ResponseWriter, r *http.Request) {
	crawler.CacheMutex.RLock()
	result := crawler.Cache
	crawler.CacheMutex.RUnlock()

	var list []model.Product
	for _, p := range result.Products {
		if p.IsOnSale {
			list = append(list, p)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(model.APIResponse{
		Code: 200, Message: "success",
		Data: map[string]interface{}{
			"time_slots": result.TimeSlots, "products": list,
			"on_sale_count": len(list), "updated_at": result.UpdatedAt,
		},
	})
}
