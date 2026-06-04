package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ============================================================
// 配置结构体 - 对应 config.json
// ============================================================

// Config 总配置
type Config struct {
	Server     ServerConfig     `json:"server"`
	Crawl      CrawlConfig      `json:"crawl"`
	ServerChan ServerChanConfig `json:"serverchan"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port string `json:"port"`
}

// CrawlConfig 爬虫配置
type CrawlConfig struct {
	TargetURL string `json:"target_url"`
	Interval  int    `json:"interval"`
}

// ServerChanConfig Server酱³ 推送配置
type ServerChanConfig struct {
	UID     string `json:"uid"`
	SendKey string `json:"sendkey"`
}

// 全局配置实例
var AppConfig Config

// ============================================================
// 加载配置
// ============================================================

// Load 从指定路径加载配置文件（自动尝试多个路径）
func Load(path string) error {
	// 候选路径列表
	candidates := []string{path}

	// 可执行文件目录
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), path))
	}

	// 从当前目录向上搜索
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, path))
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}

	// 去重尝试
	var lastErr error
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		data, err := os.ReadFile(c)
		if err == nil {
			if err := json.Unmarshal(data, &AppConfig); err != nil {
				return fmt.Errorf("解析配置文件失败: %w", err)
			}
			fmt.Printf("[配置] 配置加载完成 (路径: %s)\n", c)
			goto defaults
		}
		lastErr = err
	}
	return fmt.Errorf("读取配置文件失败: %w", lastErr)

defaults:
	// 补全默认值
	if AppConfig.Server.Port == "" {
		AppConfig.Server.Port = ":8008"
	}
	if AppConfig.Crawl.TargetURL == "" {
		AppConfig.Crawl.TargetURL = "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0"
	}
	if AppConfig.Crawl.Interval <= 0 {
		AppConfig.Crawl.Interval = 3
	}

	fmt.Printf("[配置] 配置加载完成 (端口: %s, 爬取间隔: %d分钟)\n", AppConfig.Server.Port, AppConfig.Crawl.Interval)
	return nil
}

// CrawlInterval 返回爬取间隔
func CrawlInterval() time.Duration {
	return time.Duration(AppConfig.Crawl.Interval) * time.Minute
}

// ServerChanEnabled 判断推送是否已配置
func ServerChanEnabled() bool {
	return AppConfig.ServerChan.UID != "" && AppConfig.ServerChan.SendKey != ""
}
