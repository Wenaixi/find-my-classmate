# syntax=docker/dockerfile:1
# 多阶段构建：前端产物嵌入 Go 二进制，单一端口 3078 暴露

# ---- 阶段 1：构建前端 ----
FROM node:20-alpine AS frontend
WORKDIR /build
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build
# 产物位于 /build/server/web

# ---- 阶段 2：构建后端 ----
FROM golang:1.26-alpine AS backend
WORKDIR /build
COPY server/ ./server/
# 将前端产物拷入 Go 构建上下文（web.go 通过 //go:embed all:web 读取）
COPY --from=frontend /build/server/web ./server/web
WORKDIR /build/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/findmyclassmate .

# ---- 阶段 3：运行 ----
FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=backend /out/findmyclassmate /app/findmyclassmate
# 数据与日志目录：启动自举（服务也会幂等创建），此处显式建好并授权
RUN mkdir -p /app/data /tmp/fmc && chown -R app:app /app /tmp/fmc
USER app
ENV PORT=3078
ENV FMC_DATA_DIR=/app/data
ENV FMC_LOG_DIR=/tmp/fmc
EXPOSE 3078
VOLUME ["/app/data"]
ENTRYPOINT ["/app/findmyclassmate"]
