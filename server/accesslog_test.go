package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// accessLog 的输出是隐私红线的唯一出口：脱敏后的客户端 IP 与不含查询串的路径
// 都只在这里产生（middleware.go 的 logInfof 实参位）。ip_test.go 把 maskedIP
// 函数本身测得很完整，但那只证明函数正确，不证明 accessLog 真的调用了它——
// 变异把 maskedIP(r.RemoteAddr) 换成 r.RemoteAddr 后全量测试零翻红。
// 本用例从 accessLog 的出口断言日志正文，把「实现与断言对齐」这件事钉住。
//
// 双向断言缺一不可：
//   - 只断言「日志里是脱敏形态」：删掉整条 IP 输出后该形态消失，仍会翻红，但
//     若实现改为记录其他内容则可能蒙混；正向断言锁定输出形态。
//   - 只断言「完整 IP 不出现」：实现完全不记录 IP 时同样通过，钉不住脱敏。
//
// 刻意不复用「不记录查询参数」这一侧作为本用例的承重：healthlog_test.go
// 已断言普通请求被记录，但那是存在性与条数。本用例只对 IP 负责。
func TestAccessLogMasksClientIP(t *testing.T) {
	buf := captureLogs(t)

	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// 显式指定 RemoteAddr，让断言意图自明而非依赖 httptest 的默认值。
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil)
	req.RemoteAddr = "203.0.113.45:54321"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entry := buf.String()
	if !strings.Contains(entry, "access") {
		t.Fatalf("普通请求应记录访问日志，实际: %q", entry)
	}

	// 正向：日志中的 IP 必须是脱敏形态。
	if !strings.Contains(entry, "203.0.*.*") {
		t.Errorf("访问日志应记录脱敏后的 IP（203.0.*.*），实际: %q", entry)
	}

	// 反向：完整 IP 的后两段不得出现。删掉脱敏、改打原始 RemoteAddr 时，
	// "113.45" 会出现在日志里，本条立即翻红。
	if strings.Contains(entry, "113.45") {
		t.Errorf("访问日志泄露了完整 IP 的后两段，实际: %q", entry)
	}
}

// accessLog 声明「不记录查询参数与响应内容」。查询串这一侧同样只有注释承诺：
// healthlog_test.go 断言的是日志的存在性与条数，从不看日志正文，
// 因此把路径改为记录 RequestURI（含 query）时无任何用例会翻红。
// 本用例从出口断言查询串不出现。
//
// 双向：正向断言路径被记录（证明用例测的不是「什么都没记录」），
// 反向断言查询串不出现。
func TestAccessLogOmitsQueryString(t *testing.T) {
	buf := captureLogs(t)

	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王小红&limit=50", nil))

	entry := buf.String()
	// 正向：路径本身必须被记录，否则下面的反向断言形同虚设。
	if !strings.Contains(entry, "/api/search") {
		t.Fatalf("访问日志应记录请求路径，实际: %q", entry)
	}
	// 反向：查询串不得进入日志。accessLog 用的是 r.URL.Path 而非 RequestURI，
	// 这条断言承重该选择。
	if strings.Contains(entry, "q=") || strings.Contains(entry, "王小红") {
		t.Errorf("访问日志记录了查询参数（隐私红线），实际: %q", entry)
	}
}

// IPv6 客户端的脱敏形态与 IPv4 不同：保留前两组 + :::*，而非前两段。
// 该分支的脱敏正确性由 ip_test.go 覆盖，但「accessLog 确实经由 maskedIP」
// 对 IPv4 与 IPv6 是同一条路径；本用例防止将来把 IPv6 改成原样输出时
// 只被 IPv4 用例挡不住。
func TestAccessLogMasksIPv6ClientIP(t *testing.T) {
	buf := captureLogs(t)

	handler := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	req.RemoteAddr = "[2001:db8:85a3:0:0:8a2e:370:7334]:443"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	entry := buf.String()
	if !strings.Contains(entry, "2001:db8:::*") {
		t.Errorf("IPv6 访问日志应保留前两组并遮蔽其余，实际: %q", entry)
	}
	if strings.Contains(entry, "8a2e") || strings.Contains(entry, "7334") {
		t.Errorf("IPv6 访问日志泄露了后续分组，实际: %q", entry)
	}
}
