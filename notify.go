package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// Server酱³ 消息推送
// API文档: https://sct.ftqq.com/
// 新格式: https://sctapi.ftqq.com/<sendkey>.send?title=xxx&desp=xxx
// ============================================================

// ServerChanResponse 推送响应
type ServerChanResponse struct {
	Code    int    `json:"code"`
	Errno   int    `json:"errno"`
	Message string `json:"message"`
}

// SendServerChan 发送 Server酱 推送（GET方式）
// API文档: https://sct.ftqq.com/ (新一代Server酱)
// 新格式: https://sctapi.ftqq.com/<sendkey>.send?title=xxx&desp=xxx
func SendServerChan(title, desp string) {
	if !ServerChanEnabled() {
		return
	}

	// 兼容两种 sendkey 格式：
	// 1. 完整URL（自定义推送服务）
	// 2. 仅key（使用官方API）
	apiURL := appConfig.ServerChan.SendKey
	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		apiURL = fmt.Sprintf("https://sctapi.ftqq.com/%s.send",
			appConfig.ServerChan.SendKey,
		)
	}

	// Server酱 使用 GET + query 参数方式更稳定
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("[错误] Server酱 请求创建失败: %v", err)
		return
	}

	q := req.URL.Query()
	q.Add("title", title)
	q.Add("desp", desp)
	q.Add("tags", "Roco-API")
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[错误] Server酱 推送请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var result ServerChanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[错误] Server酱 响应解析失败: %v", err)
		return
	}

	if result.Code == 0 || result.Errno == 0 {
		log.Println("[完成] Server酱 推送成功")
	} else {
		log.Printf("[警告] Server酱 推送失败: %s (code=%d)", result.Message, result.Code)
	}
}
