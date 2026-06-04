package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"roco-api/internal/config"
)

// ============================================================
// Server酱³ 消息推送
// API: https://<uid>.push.ft07.com/send/<sendkey>.send
// 文档: https://doc.sc3.ft07.com/zh/serverchan3/server/api
// ============================================================

// ServerChanMessage 推送消息结构
type ServerChanMessage struct {
	Title string `json:"title"`           // 标题（必填）
	Desp  string `json:"desp,omitempty"`  // 正文（可选，支持 Markdown）
	Tags  string `json:"tags,omitempty"`  // 标签（可选，竖线分隔）
	Short string `json:"short,omitempty"` // 简短描述（可选）
}

// ServerChanResponse 推送响应
type ServerChanResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Send 发送 Server酱³ 推送（POST JSON 方式）
func Send(title, desp string) {
	if !config.ServerChanEnabled() {
		return // 未配置，静默跳过
	}

	apiURL := fmt.Sprintf("https://%s.push.ft07.com/send/%s.send",
		config.AppConfig.ServerChan.UID,
		config.AppConfig.ServerChan.SendKey,
	)

	msg := ServerChanMessage{
		Title: title,
		Desp:  desp,
		Tags:  "Roco-API",
		Short: title,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("❌ Server酱 消息序列化失败: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("❌ Server酱 推送请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var result ServerChanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("❌ Server酱 响应解析失败: %v", err)
		return
	}

	if result.Code == 0 {
		log.Println("✅ Server酱 推送成功")
	} else {
		log.Printf("⚠️ Server酱 推送失败: %s (code=%d)", result.Message, result.Code)
	}
}

// SendGET 使用 GET 方式发送 Server酱³ 推送（备选方案）
func SendGET(title, desp string) {
	if !config.ServerChanEnabled() {
		return
	}

	apiURL := fmt.Sprintf("https://%s.push.ft07.com/send/%s.send?title=%s&desp=%s",
		config.AppConfig.ServerChan.UID,
		config.AppConfig.ServerChan.SendKey,
		url.QueryEscape(title),
		url.QueryEscape(desp),
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		log.Printf("❌ Server酱(GET) 推送请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var result ServerChanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("❌ Server酱(GET) 响应解析失败: %v", err)
		return
	}

	if result.Code == 0 {
		log.Println("✅ Server酱(GET) 推送成功")
	} else {
		log.Printf("⚠️ Server酱(GET) 推送失败: %s (code=%d)", result.Message, result.Code)
	}
}
