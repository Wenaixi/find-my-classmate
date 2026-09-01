package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// F27+F61：直序列化 SearchResponse 时 Student 的 NameKey 必须被排除（隐私红线由类型保证）
func TestSearchResponseDirectSerializeNoNameKey(t *testing.T) {
	resp := SearchResponse{
		Items: []Student{
			{Name: "张三", NameKey: "张三", Grade: GradeOne, ClassName: "1班"},
		},
		Total: 1, Limit: 10, Offset: 0, HasMore: false,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	items := m["items"].([]any)
	item := items[0].(map[string]any)
	for _, banned := range []string{"NameKey", "Name", "ClassName", "Grade"} {
		if _, ok := item[banned]; ok {
			t.Errorf("直序列化不应含字段 %s（隐私红线）", banned)
		}
	}
	for _, key := range []string{"name", "grade", "class"} {
		if _, ok := item[key]; !ok {
			t.Errorf("直序列化应含字段 %s", key)
		}
	}
}

// F61：/api/search 响应与直序列化等价（toResponse 删除后由结构体直接输出）
func TestSearchResponseJSONContract(t *testing.T) {
	resp := SearchResponse{
		Items: []Student{{Name: "张三", NameKey: "张三", Grade: GradeOne, ClassName: "1班"}},
		Total: 1, Limit: 10, Offset: 0, HasMore: false,
	}
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, resp)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["total"] != float64(1) || m["hasMore"] != false {
		t.Errorf("响应结构异常: %#v", m)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}
