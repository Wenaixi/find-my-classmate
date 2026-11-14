package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rosterJSON 构造一个单学校 roster 文档，减少用例里的字符串噪音。
// gradeClasses 形如 {"高一": {"一班": {"张三", "李四"}}}。
func rosterJSON(school string, gradeClasses map[string]map[string][]string) string {
	grades := make([]string, 0, len(gradeClasses))
	for grade, classes := range gradeClasses {
		classParts := make([]string, 0, len(classes))
		for class, names := range classes {
			nameParts := make([]string, 0, len(names))
			for _, n := range names {
				nameParts = append(nameParts, `{"姓名":"`+n+`"}`)
			}
			classParts = append(classParts, `{"班名":"`+class+`","学生":[`+strings.Join(nameParts, ",")+`]}`)
		}
		grades = append(grades, `{"年级名":"`+grade+`","班级":[`+strings.Join(classParts, ",")+`]}`)
	}
	return `{"区县":"福清市","学校":[{"校名":"` + school + `","年级":[` + strings.Join(grades, ",") + `]}]}`
}

func writeRoster(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 单文件内含多所学校：两校学生都必须加载，且各自 school 正确。
func TestLoadStudentsMultiSchoolSingleFile(t *testing.T) {
	dir := t.TempDir()
	doc := `{"区县":"福清市","学校":[` +
		`{"校名":"福清一中","年级":[{"年级名":"高一","班级":[{"班名":"一班","学生":[{"姓名":"甲"}]}]}]},` +
		`{"校名":"福清二中","年级":[{"年级名":"高二","班级":[{"班名":"二班","学生":[{"姓名":"乙"}]}]}]}]}`
	writeRoster(t, dir, "both.json", doc)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("多学校单文件应可加载: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("应加载 2 条，实际 %d", len(students))
	}
	seen := map[string]string{}
	for _, s := range students {
		seen[s.Name] = s.School
	}
	if seen["甲"] != "福清一中" || seen["乙"] != "福清二中" {
		t.Errorf("学校归属错误: %#v", seen)
	}
}

// 跨文件校名重复必须报错（校名是唯一键），并指明文件。
func TestLoadStudentsDuplicateSchoolAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"甲"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("福清一中", map[string]map[string][]string{"高二": {"二班": {"乙"}}}))
	_, err := loadStudents(dir)
	if err == nil || !strings.Contains(err.Error(), "校名重复") {
		t.Fatalf("跨文件校名重复应报错，实际 %v", err)
	}
}

// 校名为空必须拒绝。
func TestLoadStudentsRejectsEmptySchoolName(t *testing.T) {
	dir := t.TempDir()
	doc := `{"区县":"福清市","学校":[{"校名":"","年级":[{"年级名":"高一","班级":[{"班名":"一班","学生":[{"姓名":"甲"}]}]}]}]}`
	writeRoster(t, dir, "a.json", doc)
	_, err := loadStudents(dir)
	if err == nil || !strings.Contains(err.Error(), "缺少校名") {
		t.Fatalf("校名为空应报错，实际 %v", err)
	}
}

// 年级名为空必须拒绝。
func TestLoadStudentsRejectsEmptyGradeName(t *testing.T) {
	dir := t.TempDir()
	doc := `{"区县":"福清市","学校":[{"校名":"福清一中","年级":[{"年级名":"","班级":[{"班名":"一班","学生":[{"姓名":"甲"}]}]}]}]}`
	writeRoster(t, dir, "a.json", doc)
	_, err := loadStudents(dir)
	if err == nil || !strings.Contains(err.Error(), "缺少年级名") {
		t.Fatalf("年级名为空应报错，实际 %v", err)
	}
}

// 班名为空必须拒绝。
func TestLoadStudentsRejectsEmptyClassName(t *testing.T) {
	dir := t.TempDir()
	doc := `{"区县":"福清市","学校":[{"校名":"福清一中","年级":[{"年级名":"高一","班级":[{"班名":"","学生":[{"姓名":"甲"}]}]}]}]}`
	writeRoster(t, dir, "a.json", doc)
	_, err := loadStudents(dir)
	if err == nil || !strings.Contains(err.Error(), "缺少班名") {
		t.Fatalf("班名为空应报错，实际 %v", err)
	}
}

// 跨校同名同班学生必须都保留（去重键含学校）。
func TestLoadStudentsKeepsSameNameAcrossSchools(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("福清二中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("跨校同名应可加载: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("跨校同名同班应保留 2 条（去重键含学校），实际 %d", len(students))
	}
}

// 学校加载序按文件名排序后的首次出现序，SchoolIdx 单调递增。
func TestLoadStudentsSchoolOrderFollowsFilename(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "b_school.json", rosterJSON("乙校", map[string]map[string][]string{"高一": {"一班": {"乙生"}}}))
	writeRoster(t, dir, "a_school.json", rosterJSON("甲校", map[string]map[string][]string{"高一": {"一班": {"甲生"}}}))
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	idx := map[string]int{}
	for _, s := range students {
		idx[s.School] = s.SchoolIdx
	}
	if idx["甲校"] != 0 || idx["乙校"] != 1 {
		t.Errorf("SchoolIdx 应随文件名排序递增（a_school=0, b_school=1），实际 %#v", idx)
	}
}

// SearchInSchool 精确过滤：只返回目标学校。
func TestSearchInSchoolFiltersExactly(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三", "李四"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("福清二中", map[string]map[string][]string{"高一": {"一班": {"张三", "王五"}}}))
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := SearchInSchool(students, "张三", "福清一中", 10, 0)
	if got.Total != 1 || got.Items[0].School != "福清一中" {
		t.Fatalf("按校过滤应只命中福清一中 1 条，实际 total=%d %#v", got.Total, got.Items)
	}
}

// 空串 school 等于不过滤。
func TestSearchInSchoolEmptyMeansAll(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("福清二中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	students, _ := loadStudents(dir)
	got := SearchInSchool(students, "张三", "", 10, 0)
	if got.Total != 2 {
		t.Fatalf("空 school 应返回全部 2 条，实际 %d", got.Total)
	}
}

// 不存在的学校名过滤后为空，不得退化为全校。
func TestSearchInSchoolUnknownSchoolReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	students, _ := loadStudents(dir)
	got := SearchInSchool(students, "张三", "不存在的学校", 10, 0)
	if got.Total != 0 {
		t.Fatalf("不存在学校应返回 0，实际 %d", got.Total)
	}
}

// 关键守卫：只有学校、没有任何姓名/年段/班级条件时，仍必须返回空（不得浏览全校）。
func TestSearchWithSchoolOnlyReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三", "李四"}}}))
	students, _ := loadStudents(dir)
	for _, q := range []string{"", "  ", "，", "、"} {
		if got := SearchInSchool(students, q, "福清一中", 10, 0); got.Total != 0 {
			t.Errorf("q=%q 且仅有学校条件时必须返回空（防全量），实际 %d", q, got.Total)
		}
	}
}

// 学校 + 姓名 + 年段 + 班级组合过滤。
func TestSearchInSchoolCombinedConditions(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{
		"高一": {"一班": {"张三"}, "二班": {"张三"}},
		"高二": {"一班": {"张三"}},
	}))
	students, _ := loadStudents(dir)
	if got := SearchInSchool(students, "张三 高一", "福清一中", 10, 0); got.Total != 2 {
		t.Errorf("学校+年段应命中 2 条，实际 %d", got.Total)
	}
	if got := SearchInSchool(students, "张三 高一 二班", "福清一中", 10, 0); got.Total != 1 {
		t.Errorf("学校+年段+班级应命中 1 条，实际 %d", got.Total)
	}
}

// schoolList 只列含高中年级（高一~高三）的学校；小学数据不出现。
func TestSchoolListOnlyHighSchools(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "high.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	writeRoster(t, dir, "primary.json", rosterJSON("某小学", map[string]map[string][]string{"一年级": {"一班": {"小明"}}}))
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 2 {
		t.Fatalf("小学数据也应加载（共 2 条），实际 %d", len(students))
	}
	list := schoolList(students)
	if len(list) != 1 || list[0] != "福清一中" {
		t.Fatalf("下拉只应含高中校 [福清一中]，实际 %#v", list)
	}
}

// schoolList 顺序与加载序一致。
func TestSchoolListOrderStable(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("甲校", map[string]map[string][]string{"高一": {"一班": {"甲"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("乙校", map[string]map[string][]string{"高一": {"一班": {"乙"}}}))
	students, _ := loadStudents(dir)
	list := schoolList(students)
	if len(list) != 2 || list[0] != "甲校" || list[1] != "乙校" {
		t.Fatalf("下拉顺序应为 [甲校 乙校]，实际 %#v", list)
	}
}

// 全部为小学时必须返回空下拉（不得让小学进下拉）。
func TestSchoolListEmptyWhenNoHighSchool(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "p.json", rosterJSON("某小学", map[string]map[string][]string{"一年级": {"一班": {"小明"}}}))
	students, _ := loadStudents(dir)
	if list := schoolList(students); len(list) != 0 {
		t.Fatalf("无高中校时下拉应为空，实际 %#v", list)
	}
}

// GET /api/schools 返回 {schools:[...]}。
func TestAPISchoolsEndpoint(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	store, err := newStudentStore(dir, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/schools", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", rec.Code)
	}
	var body struct {
		Schools []string `json:"schools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体应为 JSON: %v", err)
	}
	if len(body.Schools) != 1 || body.Schools[0] != "福清一中" {
		t.Errorf("schools = %#v，期望 [福清一中]", body.Schools)
	}
}

// /api/schools 只接受 GET。
func TestAPISchoolsMethodNotAllowed(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	store, _ := newStudentStore(dir, time.Now)
	mux := buildMux(store, "dev")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/schools", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("状态 = %d，期望 405", rec.Code)
	}
}

// 数据不可用时 /api/schools 报 data_unavailable。
func TestAPISchoolsDataUnavailable(t *testing.T) {
	store, dir := newTestStoreWithDir(t, map[string]string{
		"a.json": rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}),
	}, time.Now)
	mux := buildMux(store, "dev")
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 推进超过探测窗口，使损坏被观察
	time.Sleep(1100 * time.Millisecond)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/schools", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态 = %d，期望 500", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "data_unavailable" {
		t.Errorf("error = %q，期望 data_unavailable", body["error"])
	}
}

// /api/search 的 school 参数生效；过长触发 invalid_query。
func TestAPISearchSchoolParameter(t *testing.T) {
	dir := t.TempDir()
	writeRoster(t, dir, "a.json", rosterJSON("福清一中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	writeRoster(t, dir, "b.json", rosterJSON("福清二中", map[string]map[string][]string{"高一": {"一班": {"张三"}}}))
	store, _ := newStudentStore(dir, time.Now)
	mux := buildMux(store, "dev")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=%E5%BC%A0%E4%B8%89&school=%E7%A6%8F%E6%B8%85%E4%B8%80%E4%B8%AD", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态 = %d，期望 200", rec.Code)
	}
	var body struct {
		Total int       `json:"total"`
		Items []Student `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].School != "福清一中" {
		t.Errorf("school 过滤应命中福清一中 1 条，实际 total=%d %#v", body.Total, body.Items)
	}

	// 超长 school → invalid_query(400)
	long := strings.Repeat("校", maxQueryRunes+1)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/search?q=%E5%BC%A0&school="+long, nil))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("超长 school 状态 = %d，期望 400", rec2.Code)
	}
}

// 排序：同分时按学校加载序（SchoolIdx），未知学校排最后。
func TestSearchSortsBySchoolLayer(t *testing.T) {
	students := []Student{
		newStudentSchool("乙校", "张同", GradeOne, "一班", parseClassName("一班"), 1),
		newStudentSchool("甲校", "张同", GradeOne, "一班", parseClassName("一班"), 0),
		newStudent("张同", GradeOne, "一班", parseClassName("一班")), // 无学校：SchoolIdx=-1 排最后
	}
	got := Search(students, "张同", 10, 0)
	if len(got.Items) != 3 {
		t.Fatalf("应命中 3 条，实际 %d", len(got.Items))
	}
	if got.Items[0].School != "甲校" || got.Items[1].School != "乙校" || got.Items[2].School != "" {
		t.Errorf("排序应为 甲校→乙校→无学校，实际 %#v", []string{got.Items[0].School, got.Items[1].School, got.Items[2].School})
	}
}