package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const validGradeOne = `{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`
const validGradeTwo = `{"标题":"福清一中2025级高二编班名单","名单":{"2班":[{"姓名":"李四"}]}}`

// health 应反映数据可用性——数据损坏时返回 503
func TestHealthReflectsDataAvailability(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(validGradeOne), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(validGradeTwo), 0o644)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("健康数据 health 应为 200，实际 %d", rec.Code)
	}

	// 破坏数据文件，并推进超过探测窗口，使探测真实命中
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte("{"+"broken"), 0o644)
	clock.advance(2 * time.Second)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("数据损坏时 health 应为 503，实际 %d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "degraded" {
		t.Errorf("health 损坏态 status = %q，期望 degraded", body["status"])
	}

	// 恢复后 health 应回到 200
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(validGradeTwo), 0o644)
	clock.advance(2 * time.Second)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("恢复后 health 应为 200，实际 %d", rec.Code)
	}
}

// 未知 /api/* 路径应返回 JSON 404（not_found），而非 text/plain
func TestUnknownAPIJSON404(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/typo", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/api/typo 状态 = %d，期望 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q，期望 application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体应为 JSON，实际 %q", rec.Body.String())
	}
	if body["error"] != "not_found" {
		t.Errorf("error 码 = %q，期望 not_found", body["error"])
	}
}

// /api/search 响应键集合必须只含 name/grade/class（无 NameKey）
func TestSearchResponseKeys(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=王", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items 应为 1 条，实际 %#v", body["items"])
	}
	item := items[0].(map[string]any)
	for _, key := range []string{"name", "grade", "class"} {
		if _, ok := item[key]; !ok {
			t.Errorf("响应缺字段 %s", key)
		}
	}
	for _, banned := range []string{"NameKey", "Name", "ClassName", "Grade", "ClassNo", "GradeIdx"} {
		if _, ok := item[banned]; ok {
			t.Errorf("响应不应含字段 %s（隐私红线）", banned)
		}
	}
}

// 数据损坏时 /api/search 应 500 data_unavailable
func TestSearchDataUnavailable(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store, "dev")
	_ = os.WriteFile(filepath.Join(store.dir, "高二.json"), []byte("{"+"bad"), 0o644)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=李", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("数据损坏 search 状态 = %d，期望 500", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "data_unavailable" {
		t.Errorf("error 码 = %q，期望 data_unavailable", body["error"])
	}
}

// /api/search 非 GET 应返回 405 且带 Allow: GET
func TestSearchMethodNotAllowed(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/search", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/search 状态 = %d，期望 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Errorf("Allow = %q，期望 GET", allow)
	}
}

// TestSearchRejectsOutOfRangeParameters 覆盖 api.go 的参数解析 400 路径。
//
// 变异事实（2026-09-26 第六轮）：把这三条判定改成恒假（parseErr != nil && false）
// 后全仓后端测试仍全绿。errors_test.go 虽锁住了错误码到 HTTP 状态的映射表，
// 但不经由参数解析——「用户传 ?limit=abc」这类最典型的畸形输入此前零覆盖。
func TestSearchRejectsOutOfRangeParameters(t *testing.T) {
	cases := []struct {
		name  string
		query string
		code  string
	}{
		{"limit 越界", "?q=%E7%8E%8B&limit=999", errCodeInvalidLimit},
		{"limit 非数字", "?q=%E7%8E%8B&limit=abc", errCodeInvalidLimit},
		{"offset 为负", "?q=%E7%8E%8B&offset=-1", errCodeInvalidOffset},
		{"offset 非数字", "?q=%E7%8E%8B&offset=xyz", errCodeInvalidOffset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t, map[string]string{"高一.json": validGradeOne})
			req := httptest.NewRequest(http.MethodGet, "/api/search"+tc.query, nil)
			rec := httptest.NewRecorder()
			buildMux(store, "test").ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("状态 = %d，期望 400", rec.Code)
			}
			var body struct{ Error string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != tc.code {
				t.Errorf("错误码 = %q，期望 %q", body.Error, tc.code)
			}
		})
	}
}

// TestSearchRejectsOverlongQuery 覆盖 query 长度上限的 400 路径。
// 输入按 rune 计数而非字节：上限本身以 rune 判定，用多字节中文才测得到边界。
func TestSearchRejectsOverlongQuery(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne})
	overlong := strings.Repeat("王", maxQueryRunes+1)
	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	req.URL.RawQuery = "q=" + overlong
	rec := httptest.NewRecorder()
	buildMux(store, "test").ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态 = %d，期望 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), errCodeInvalidQuery) {
		t.Errorf("响应体 = %q，期望含 %q", rec.Body.String(), errCodeInvalidQuery)
	}
}

// TestSearchAcceptsBoundaryParameters 锁住合法边界不被误伤：
// limit=1、limit=maxLimit、offset=0 必须正常返回 200。
// 与上面的拒绝用例成对，防止「一律拒绝」这种同样能过测试的错误实现。
func TestSearchAcceptsBoundaryParameters(t *testing.T) {
	queries := []string{
		"?q=%E7%8E%8B&limit=1",
		"?q=%E7%8E%8B&limit=" + strconv.Itoa(maxLimit),
		"?q=%E7%8E%8B&offset=0",
	}
	for _, query := range queries {
		store := newTestStore(t, map[string]string{"高一.json": validGradeOne})
		req := httptest.NewRequest(http.MethodGet, "/api/search"+query, nil)
		rec := httptest.NewRecorder()
		buildMux(store, "test").ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("合法参数 %s 状态 = %d，期望 200", query, rec.Code)
		}
	}
}
