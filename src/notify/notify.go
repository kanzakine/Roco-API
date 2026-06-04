package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"roco-api/config"
)

// chanCreds 获取推送凭证：优先读环境变量，其次 config.json
func chanCreds() (uid, key string) {
	uid = os.Getenv("CHAN_UID")
	key = os.Getenv("CHAN_KEY")
	if uid == "" {
		uid = config.AppConfig.ServerChan.UID
	}
	if key == "" {
		key = config.AppConfig.ServerChan.SendKey
	}
	return
}

// Send 发送 Server酱³ 推送
func Send(title, desp string) {
	uid, key := chanCreds()
	if uid == "" || key == "" {
		return
	}

	apiURL := fmt.Sprintf("https://%s.push.ft07.com/send/%s.send", uid, key)

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
