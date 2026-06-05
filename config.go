package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ============================================================
// 配置结构体 - 对应 config.json
// ============================================================

// Config 总配置
type Config struct {
	Server      ServerConfig      `json:"server"`
	Crawl       CrawlConfig       `json:"crawl"`
	ServerChan  ServerChanConfig  `json:"serverchan"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port string `json:"port"`
}

// CrawlConfig 爬虫配置
type CrawlConfig struct {
	TargetURL string `json:"target_url"`
	Interval  int    `json:"interval"` // 间隔（分钟）
}

// ServerChanConfig Server酱³ 推送配置
type ServerChanConfig struct {
	UID     string `json:"uid"`
	SendKey string `json:"sendkey"`
}

// 全局配置实例
var appConfig Config

// ============================================================
// 加载配置
// ============================================================

// LoadConfig 从 config.json 加载配置
func LoadConfig(path string) error {
	// 尝试多个路径（用于兼容 dlv debug 和工作目录不同）
	candidates := []string{path, "../" + path, "./config/" + path, "../config/" + path}
	var data []byte
	var err error
	for _, c := range candidates {
		data, err = os.ReadFile(c)
		if err == nil {
			path = c
			break
		}
	}
	if err != nil {
		return fmt.Errorf("读取配置文件失败（已在多个路径搜索）: %w", err)
	}
	if err := json.Unmarshal(data, &appConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}
	if appConfig.Server.Port == "" {
		appConfig.Server.Port = ":8008"
	}
	if appConfig.Crawl.TargetURL == "" {
		appConfig.Crawl.TargetURL = "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0"
	}
	if appConfig.Crawl.Interval <= 0 {
		appConfig.Crawl.Interval = 3
	}
	fmt.Printf("[配置] 配置加载完成 (端口: %s, 爬取间隔: %d分钟)\n", appConfig.Server.Port, appConfig.Crawl.Interval)
	return nil
}

// CrawlInterval 返回爬取间隔的 time.Duration
func CrawlInterval() time.Duration {
	return time.Duration(appConfig.Crawl.Interval) * time.Minute
}

// ServerChanEnabled 判断 Server酱 推送是否已配置
func ServerChanEnabled() bool {
	u := appConfig.ServerChan.UID
	k := appConfig.ServerChan.SendKey
	return u != "" && k != "" && u != "your_uid" && k != "your_sendkey"
}
