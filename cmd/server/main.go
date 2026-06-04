package main

import (
	"fmt"
	"log"
	"net/http"

	"roco-api/internal/config"
	"roco-api/internal/crawler"
	"roco-api/internal/handler"
)

func main() {
	// 0. 加载配置文件
	if err := config.Load("config.json"); err != nil {
		log.Fatalf("❌ %v", err)
	}

	// 提示推送状态
	if config.ServerChanEnabled() {
		fmt.Println("🔔 Server酱 推送已启用")
	} else {
		fmt.Println("🔕 Server酱 推送未配置（如需启用，请填写 config.json 中的 uid 和 sendkey）")
	}

	// 1. 初始化推送追踪
	crawler.InitTracker()

	// 2. 启动时立即爬取一次
	fmt.Println("🔄 首次爬取远行商人数据...")
	crawler.Do()

	// 3. 后台定时爬取
	go crawler.StartCron()

	// 4. 注册 HTTP 路由
	handler.Start()

	// 5. 启动 HTTP 服务
	port := config.AppConfig.Server.Port
	fmt.Println("\n========================================")
	fmt.Printf("🚀 API 服务已启动: http://localhost%s\n", port)
	fmt.Println("   可用接口:")
	fmt.Println("     GET  /              - 服务状态页")
	fmt.Println("     GET  /api/products  - 所有商品数据 (JSON)")
	fmt.Println("     GET  /api/onsale    - 仅在售商品 (JSON)")
	fmt.Println("========================================")
	fmt.Printf("⏰ 每 %d 分钟自动爬取更新数据\n", config.AppConfig.Crawl.Interval)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatalf("❌ 服务器启动失败: %v", err)
	}
}
