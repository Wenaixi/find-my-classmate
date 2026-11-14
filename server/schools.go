package main

import (
	"net/http"
)

// schoolsHandler 返回学校下拉候选列表：只含至少有一名高中年级学生的学校，
// 小学（仅一年级~六年级之名）不出现在下拉里，但其学生仍可被全局搜索命中
// （由 Search 的 q 过滤，不经 /api/schools 暴露为可选项）。
// 排序：按加载首现顺序（即目录文件名排序后的学校首次出现序）。
func schoolsHandler(store *studentStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, errCodeMethodNotAllowed)
			return
		}
		students, loadErr := store.view()
		if loadErr != nil {
			writeError(w, errCodeDataUnavailable)
			return
		}
		schools := schoolList(students)
		writeJSON(w, http.StatusOK, map[string]any{"schools": schools})
	}
}

// schoolList 从学生快照派生出「进入下拉的高中校名列表」。
// 规则：只保留至少有一名学生的年级属于 knownGrades（高一/高二/高三）的学校。
// 顺序 = 加载序（SchoolIdx 升序）；学校名去重。
func schoolList(students []Student) []string {
	seenSchool := make(map[string]bool)
	schoolIdx := make(map[string]int)
	order := make([]string, 0, 8)
	for _, s := range students {
		if s.School == "" {
			continue
		}
		if s.GradeIdx >= len(knownGrades) {
			// 未知年段（一年级~六年级等）：该校不因这条学生进入高中下拉
			continue
		}
		if _, ok := schoolIdx[s.School]; !ok {
			schoolIdx[s.School] = s.SchoolIdx
			order = append(order, s.School)
			seenSchool[s.School] = true
		}
	}
	// 按 SchoolIdx 排序（加载序稳定；未知 -1 排最后，但在筛选后不可能出现）
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && schoolIdx[order[j-1]] > schoolIdx[order[j]]; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
	return order
}
