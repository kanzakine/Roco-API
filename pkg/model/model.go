package model

// ShopSlot 时段信息
type ShopSlot struct {
	Label string `json:"label"` // 如 "08:00-12:00"
}

// Product 商品
type Product struct {
	Name      string `json:"name"`       // 商品名称
	Price     string `json:"price"`      // 价格
	Limit     string `json:"limit"`      // 限购
	Category  string `json:"category"`   // 分类
	Desc      string `json:"desc"`       // 描述
	ImageURL  string `json:"image_url"`  // 图片
	IsOnSale  bool   `json:"is_on_sale"` // 是否在售
	RemainStr string `json:"remain"`     // 剩余时间文本
	SlotLabel string `json:"slot_label"` // 所属时段，如 "08:00-12:00"
}

// CrawlResult 爬取结果（整个缓存）
type CrawlResult struct {
	TimeSlots   []ShopSlot `json:"time_slots"`    // 四个时段
	Products    []Product  `json:"products"`      // 所有商品
	OnSaleCount int        `json:"on_sale_count"` // 在售数量
	TotalCount  int        `json:"total_count"`   // 总商品数
	UpdatedAt   string     `json:"updated_at"`    // 最后更新时间
}

// APIResponse API 通用响应格式
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
