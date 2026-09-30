package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func resolveDataDir() string {
	if value := os.Getenv("FMC_DATA_DIR"); value != "" {
		return value
	}
	current, err := os.Getwd()
	if err != nil {
		return "./data"
	}
	if filepath.Base(current) == "server" {
		return filepath.Join(current, "..", "data")
	}
	return filepath.Join(current, "data")
}

// version 由发布流水线 ldflags 注入（-X main.version=<git tag>）；本地构建默认为 dev。
var version = "dev"

func main() {
	dataDir := resolveDataDir()
	// 启动自举：幂等创建数据目录，缺失数据文件时给出清晰指引而非静默空跑
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败 %s: %v", dataDir, err)
	}
	output, logFile, err := openLog(dataDir)
	if err != nil {
		// 容器场景数据目录通常只读挂载（Dockerfile 的 VOLUME + compose 的 :ro），
		// 而 resolveLogDir 的回退分支正是 dataDir/log——在只读挂载下它必然失败。
		// 两个真实部署都显式设了 FMC_LOG_DIR 规避，但镜像变体删掉那行 ENV 后
		// 运维只会看到「初始化日志失败」，无从知道该设哪个变量。此处补上指引，
		// 与数据目录自举的「给出清晰指引而非静默空跑」保持同一风格。
		log.Fatalf("初始化日志失败: %v（数据目录 %s 可能不可写；容器部署请设置 FMC_LOG_DIR 指向可写目录）", err, dataDir)
	}
	defer logFile.Close()
	log.SetOutput(output)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	store, err := newStudentStore(dataDir, time.Now)
	if err != nil {
		log.Fatal(err)
	}
	logInfof("loaded %d students", store.Size())

	mux := buildMux(store, version)
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	logInfof("FindMyClassmate %s listening on :%s", version, port)
	server := buildServer(":"+port, newHandlerChain(mux))
	log.Fatal(server.ListenAndServe())
}

// newHandlerChain 组装 production 请求链，是中间件顺序的唯一事实源。
// 顺序（由外到内）：securityHeaders → rateLimit → accessLog → mux。
//
// 安全头居最外是刻意的：全站响应头契约必须覆盖所有响应，包括限流拒绝
// （429）与被访问日志跳过的健康检查；限流在 accessLog 之外短路，
// 被拒绝的请求不写访问日志，避免攻击流量放大日志磁盘写入。
// 429 因位于 securityHeaders 内侧，响应头由外层统一设置，writeRateLimited
// 无需手工重放。
func newHandlerChain(mux http.Handler) http.Handler {
	return newHandlerChainWith(mux, newRateLimiter(rateCapacity, rateInterval, time.Now))
}

// newHandlerChainWith 是链装配的唯一实现：只替换限流器本身，链的顺序与
// 其余中间件行为全部走 production 代码路径，调用方（含测试）无需复制嵌套。
// 委托关系取代此前测试侧重写一遍嵌套的做法——链顺序改错时测试会跟着错，
// 平行实现会让四个组合政策测试在测另一条链却依然全绿。
func newHandlerChainWith(mux http.Handler, limiter *rateLimiter) http.Handler {
	return securityHeaders(rateLimitWith(limiter, accessLog(mux)))
}

// 服务器超时常量。刻意不进 config.go：那个文件的边界是「能与前端对拍的
// 跨语言契约常量」，而这四个值只有 Go 一侧（config.go 的 defaultPort 同理，
// 属部署形态而非契约）。命名的价值在于消除「实现写字面量、测试复述字面量」
// 这一对：此前 server_test.go 用 10s/15s/60s/5s 复述实现里的字面量，
// 改实现与改测试可以各改一处而不冲突，断言看似承重却与实现无共同事实源。
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

// buildServer 组装带超时配置的 http.Server：防止慢速攻击挂起连接。
// 超时值被上方具名常量锁定，测试与其共用同一事实源。
func buildServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}
