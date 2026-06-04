package main

import (
	"fmt"
	"log"
	"net/http"

	"roco-api/config"
	"roco-api/src/crawler"
	"roco-api/src/handler"
)

func main() {
	// 加载配置
	if err := config.Load("config.json"); err != nil {
		log.Fatalf("[错误] %v", err)
	}

	if config.ServerChanEnabled() {
		fmt.Println("[通知] Server酱 推送已启用")
	} else {
		fmt.Println("[信息] Server酱 推送未配置（如需启用，请填写 config.json 中的 uid 和 sendkey）")
	}

	// 初始化推送追踪
	crawler.InitTracker()

	// 首次爬取
	fmt.Println("[任务] 首次爬取远行商人数据...")
	crawler.Do()

	// 定时爬取
	go crawler.StartCron()

	// 注册路由
	handler.Start()

	// 启动 HTTP
	port := config.AppConfig.Server.Port
	fmt.Println("\n========================================")
	fmt.Printf("[启动] API 服务已启动: http://localhost%s\n", port)
	fmt.Println("   可用接口:")
	fmt.Println("     GET  /              - 服务状态页")
	fmt.Println("     GET  /api/products  - 所有商品数据 (JSON)")
	fmt.Println("     GET  /api/onsale    - 仅在售商品 (JSON)")
	fmt.Println("========================================")
	fmt.Printf("[定时] 每 %d 分钟自动爬取更新数据\n", config.AppConfig.Crawl.Interval)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("[错误] 服务器启动失败: %v", err)
	}
}
