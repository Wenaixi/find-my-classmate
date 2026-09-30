package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// 日志是横切关注点：热重载（data.go）、数据不可用（api.go）与访问日志（accessLog）
// 都经由本文件的 logInfof / logErrorf 输出。此前它们定义在 main.go，
// 改日志行为必须先读懂整个装配流程——日志的归属与装配的归属被同一文件绑在一起。
// 拆出后「日志怎么写」与「服务怎么装配」各自独立。
//
// 不引入日志 interface：当前只有标准库 log 一种实现，凭空造接缝是负债而非加深。

// 日志级别：数值越大越啰嗦，logf 在 logLevel < min 时跳过。
// levelWarn 当前没有输出点（无生产代码发出 warn 级日志），但级别本身保留：
// FMC_LOG_LEVEL=warn 是对外环境契约，运维可能已在用，删掉会让该配置静默失效。
// 新增 warn 级日志时基础设施已就位，无需改动级别解析。
const (
	levelError level = iota
	levelWarn
	levelInfo
)

type level int

var logLevel = parseLogLevel(os.Getenv("FMC_LOG_LEVEL"))

func parseLogLevel(value string) level {
	switch value {
	case "error":
		return levelError
	case "warn":
		return levelWarn
	default:
		return levelInfo
	}
}

func logf(min level, format string, args ...any) {
	if logLevel < min {
		return
	}
	log.Printf(format, args...)
}

func logInfof(format string, args ...any)  { logf(levelInfo, format, args...) }
func logErrorf(format string, args ...any) { logf(levelError, format, args...) }

// resolveLogDir 优先使用 FMC_LOG_DIR 环境变量；未设置时回落到数据目录下的 log 子目录。
//
// 回退分支只覆盖**本地开发**（数据目录可写），不覆盖容器场景：容器里数据目录
// 通常只读挂载（Dockerfile 的 VOLUME + compose 的 :ro），此时该回退必然导致
// openLog 失败、服务不启动。两个真实部署都显式设了 FMC_LOG_DIR 规避，
// 故今天不触发——但删掉那行 ENV 的镜像变体会静默落进这条死路。
// 失败时的指引由 main 的调用点给出（提示设置 FMC_LOG_DIR），不在此重复。
func resolveLogDir(dataDir string) string {
	if value := os.Getenv("FMC_LOG_DIR"); value != "" {
		return value
	}
	return filepath.Join(dataDir, "log")
}

func openLog(dataDir string) (io.Writer, *os.File, error) {
	logDir := resolveLogDir(dataDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(filepath.Join(logDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	// 文件与 stdout 双写：容器场景由 docker 收集 stdout，本地场景保留文件
	return io.MultiWriter(file, os.Stderr), file, nil
}
