# Roco-API 🏪

> 洛克王国：世界每日远行商人商品查询 API — 定时爬取商品数据，提供 RESTful JSON 接口

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

---

## 📖 简介

自动爬取【洛克王国：世界】每日远行商人的在售商品数据，并提供 HTTP API 接口供其他项目使用。

**数据来源：** [快爆工具箱 - 远行商人查询器](https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0)

---

## ✨ 功能

- ✅ 自动爬取远行商人商品数据（名称、价格、限购、分类、描述、图片）
- ✅ 智能判断商品在售/已结束状态，显示剩余时间
- ✅ 四个时段展示（08:00-12:00 / 12:00-16:00 / 16:00-20:00 / 20:00-24:00）
- ✅ 每 3 分钟后台自动刷新缓存
- ✅ 提供 RESTful JSON API 接口
- ✅ 自带 Web 状态页面

---

## 🚀 快速开始

### 前置条件

- [Go](https://go.dev/dl/) 1.26+

### 安装

```bash
git clone https://github.com/your-username/roco-api.git
cd Roco-API
go mod tidy
```

### 配置

所有配置集中保存在 `config.json` 中：
```
vim config.json
```

```json
{
  "server": {
    "port": ":8008"
  },
  "crawl": {
    "target_url": "https://www.onebiji.com/hykb_tools/comm/lkwgmerchant/preview.php?id=1&immgj=0",
    "interval": 3
  },
  "serverchan": {
    "uid": "your_uid",
    "sendkey": "your_sendkey"
  }
}
```

| 配置项 | 说明 |
|--------|------|
| `server.port` | HTTP 服务端口 |
| `crawl.target_url` | 爬取目标网址 |
| `crawl.interval` | 爬取间隔（分钟） |
| `serverchan.uid` | Server酱³ 的 UID（留空则不启用推送） |
| `serverchan.sendkey` | Server酱³ 的 SendKey（留空则不启用推送） |

### � Server酱 推送配置

1. 前往 [Server酱³ SendKey 页面](https://sc3.ft07.com/sendkey) 获取你的 `UID` 和 `SendKey`
2. 填写到 `config.json` 的 `serverchan` 字段中
3. 重启服务即可生效

> 如果 `uid` 和 `sendkey` 均为空，推送功能将自动禁用，不影响正常使用。

### 运行
(推荐安装Screen切后台运行)

#调试启动（推荐）

```bash
go run ./
```
# 或编译后运行（跨平台，自动识别系统）
go build -o Roco-API ./
./roco-API


启动后输出：

```
[配置] 配置加载完成 (端口: :8008, 爬取间隔: 3分钟)
[通知] Server酱 推送已启用
[任务] 首次爬取远行商人数据...
[完成] 爬取完成: 9 件商品, 6 件在售中
2026/06/05 16:46:35 [完成] Server酱 推送成功

========================================
[启动] API 服务已启动: http://localhost:8008
   GET /api/products  - 全部商品
   GET /api/onsale    - 在售商品
========================================
[定时] 每 3 分钟自动爬取更新数据
```


## 📡 API 文档

### 基础地址

```
http://localhost:8008
```

### `GET /` — Web 状态页

返回可视化的 HTML 页面，显示商品列表和服务状态。

### `GET /api/products` — 全部商品

返回所有商品数据（包括已结束的）。

**响应格式：**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "time_slots": [
      { "label": "08:00-12:00" },
      { "label": "12:00-16:00" },
      { "label": "16:00-20:00" },
      { "label": "20:00-24:00" }
    ],
    "products": [
      {
        "name": "紫莲刚玉",
        "price": "1000",
        "limit": "限购100",
        "category": "炼金材料",
        "desc": "比较常用的炼金材料，可以用来合成咕噜球和技能石。",
        "image_url": "https://...png",
        "is_on_sale": true,
        "remain": "02:34:35"
      }
    ],
    "on_sale_count": 2,
    "total_count": 6,
    "updated_at": "2026-06-04 21:25:24"
  }
}
```

### `GET /api/onsale` — 仅在售商品

只返回当前在售中的商品，结构与 `/api/products` 相同但 `products` 数组只包含在售项。

---

## ⚙️ 配置

在 `main.go` 顶部修改常量：

```go
const (
    targetURL      = "https://..."        // 爬取目标地址
    crawlInterval  = 3 * time.Minute      // 爬取间隔
    serverPort     = ":8008"              // API 服务端口
)
```

---

## 🧪 调用示例

### cURL

```bash
# 获取全部商品
curl http://localhost:8008/api/products

# 获取仅在售商品
curl http://localhost:8008/api/onsale
```

### Python

```python
import requests

resp = requests.get("http://localhost:8008/api/onsale")
data = resp.json()

for p in data["data"]["products"]:
    print(f"{p['name']} - {p['price']} - {'在售' if p['is_on_sale'] else '已结束'}")
```

### JavaScript

```javascript
fetch("http://localhost:8008/api/products")
  .then(r => r.json())
  .then(data => console.log(data.data.products));
```

---

## 📂 项目结构

```
roco-api/
├── main.go         # 主程序（爬虫 + API 服务）
├── go.mod          # Go 模块定义
├── go.sum          # 依赖锁定
└── README.md       # 本文件
```

---

## 🛠️ 技术栈

- **语言：** Go 1.26+
- **HTTP 服务：** `net/http`（标准库）
- **HTML 解析：** [goquery](https://github.com/PuerkitoBio/goquery)（jQuery 风格的 HTML 解析器）
- **数据缓存：** 内存缓存 + `sync.RWMutex` 读写锁

---

## 📄 许可证

[MIT](LICENSE)
