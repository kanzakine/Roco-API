# ============================================
# Stage 1: 构建阶段
# ============================================
FROM golang:1.26-alpine AS builder

# 使用阿里云 Go 模块代理加速
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/roco-api ./cmd/server/

# ============================================
# Stage 2: 运行阶段（最小镜像）
# ============================================
FROM alpine:3.21

# 安装 ca-certificates 用于 HTTPS 请求
RUN apk add --no-cache ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从构建阶段复制编译产物
COPY --from=builder /app/roco-api .

# 复制配置文件模板
COPY config.json .

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8008/ || exit 1

EXPOSE 8008

CMD ["./roco-api"]
