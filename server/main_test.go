package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 构造一个指向临时目录的 studentStore（TDD：数据层测试也一并覆盖）
func newTestStore(t *testing.T, files map[string]string) *studentStore {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatalf("newStudentStore: %v", err)
	}
	return store
}

const validGradeOne = `{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`
const validGradeTwo = `{"标题":"福清一中2025级高二编班名单","名单":{"2班":[{"姓名":"李四"}]}}`

// F18：health 应反映数据可用性——数据损坏时返回 503
func TestHealthReflectsDataAvailability(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(validGradeOne), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(validGradeTwo), 0o644)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	mux := buildMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("健康数据 health 应为 200，实际 %d", rec.Code)
	}

	// 破坏数据文件
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{"+"broken"), 0o644)
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
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(validGradeOne), 0o644)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("恢复后 health 应为 200，实际 %d", rec.Code)
	}
}

// F19：未知 /api/* 路径应返回 JSON 404（not_found），而非 text/plain
func TestUnknownAPIJSON404(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store)
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

// F27+F61：/api/search 响应键集合必须只含 name/grade/class（无 NameKey）
func TestSearchResponseKeys(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store)
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
	for _, banned := range []string{"NameKey", "Name", "ClassName", "Grade"} {
		if _, ok := item[banned]; ok {
			t.Errorf("响应不应含字段 %s（隐私红线）", banned)
		}
	}
}

// F18 关联：数据损坏时 /api/search 应 500 data_unavailable
func TestSearchDataUnavailable(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store)
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

// F30：/api/search 非 GET 应返回 405 且带 Allow: GET
func TestSearchMethodNotAllowed(t *testing.T) {
	store := newTestStore(t, map[string]string{"高一.json": validGradeOne, "高二.json": validGradeTwo})
	mux := buildMux(store)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/search", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/search 状态 = %d，期望 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Errorf("Allow = %q，期望 GET", allow)
	}
}
