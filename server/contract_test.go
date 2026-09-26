package main

import (
	"encoding/json"
	"os"
	"testing"
)

// contractCase 与 docs/query-contract.json 的单条记录一一对应。
type contractCase struct {
	Raw string `json:"raw"`
	// Tokens 是分词中间结果：归一后按空白切出的全部 token，在分类之前就已确定。
	// 它此前是前端独有字段、Go 侧无对应物，空白集合的分词行为只能在单侧断言；
	// 纳入契约后经 tokenize 对拍，两端各有一条承重断言。
	Tokens      []string `json:"tokens"`
	NameTokens  []string `json:"nameTokens"`
	Grade       *string  `json:"grade"`
	ClassNumber *int     `json:"classNumber"`
}

type contractFile struct {
	Cases []contractCase `json:"cases"`
}

// loadContractCases 读取跨语言解析契约的唯一事实源。
// 解析契约由前端 src/lib/query.ts 与后端 search.go 共同消费，
// 任何一侧改动解析规则都必须同步更新该文件，否则对拍测试立即失败。
func loadContractCases(t *testing.T) []contractCase {
	t.Helper()
	payload, err := os.ReadFile("../docs/query-contract.json")
	if err != nil {
		t.Fatalf("读取契约语料失败: %v", err)
	}
	var file contractFile
	if err := json.Unmarshal(payload, &file); err != nil {
		t.Fatalf("解析契约语料失败: %v", err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("契约语料不应为空")
	}
	return file.Cases
}

// TestParseQueryContractCorpus 用共享语料锁定后端解析行为。
// 语料以 Go 端实测结果为准；前端测试消费同一文件，任一侧漂移都会被捕获。
func TestParseQueryContractCorpus(t *testing.T) {
	for _, c := range loadContractCases(t) {
		t.Run(c.Raw, func(t *testing.T) {
			got := parseQuery(c.Raw)
			wantTokens := c.Tokens
			gotTokens := tokenize(c.Raw)
			if len(wantTokens) != len(gotTokens) {
				t.Fatalf("tokens = %v，期望 %v", gotTokens, wantTokens)
			}
			for i := range wantTokens {
				if wantTokens[i] != gotTokens[i] {
					t.Fatalf("tokens = %v，期望 %v", gotTokens, wantTokens)
				}
			}

			wantNames := c.NameTokens
			gotNames := got.NameTokens
			if gotNames == nil {
				gotNames = []string{}
			}
			if len(wantNames) != len(gotNames) {
				t.Fatalf("nameTokens = %v, 期望 %v", gotNames, wantNames)
			}
			for i := range wantNames {
				if wantNames[i] != gotNames[i] {
					t.Fatalf("nameTokens = %v, 期望 %v", gotNames, wantNames)
				}
			}

			wantGrade := Grade("")
			if c.Grade != nil {
				wantGrade = Grade(*c.Grade)
			}
			if got.Grade != wantGrade {
				t.Errorf("grade = %q, 期望 %q", got.Grade, wantGrade)
			}

			wantClass := 0
			if c.ClassNumber != nil {
				wantClass = *c.ClassNumber
			}
			if got.ClassNo != wantClass {
				t.Errorf("classNumber = %d, 期望 %d", got.ClassNo, wantClass)
			}
		})
	}
}
