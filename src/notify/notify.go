package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"roco-api/config"
)

// Send 发送 Server酱³ 推送
func Send(title, desp string) {
	if !config.ServerChanEnabled() {
		return
	}

	apiURL := fmt.Sprintf("https://%s.push.ft07.com/send/%s.send",
		config.AppConfig.ServerChan.UID,
		config.AppConfig.ServerChan.SendKey,
	)

	body, _ := json.Marshal(map[string]string{
		"title": title,
		"desp":  desp,
		"tags":  "Roco-API",
		"short": title,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[错误] 推送请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Code == 0 {
		log.Println("[完成] 推送成功")
	} else {
		log.Printf("[警告] 推送失败: %s (code=%d)", result.Message, result.Code)
	}
}
