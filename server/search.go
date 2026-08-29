package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Grade string

const (
	GradeOne Grade = "高一"
	GradeTwo Grade = "高二"
)

type Student struct {
	Name      string
	NameKey   string
	Grade     Grade
	ClassName string
}

type SearchResponse struct {
	Items   []Student `json:"items"`
	Total   int       `json:"total"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"hasMore"`
}

type Query struct {
	NameTokens []string
	Grade      Grade
	ClassNo    int
}

var classToken = regexp.MustCompile("^([0-9]+|[一二三四五六七八九十]+)班?$")

var classDigits = map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}

func normalizeName(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(value) {
		if unicode.IsSpace(r) || r == '\t' || r == '　' {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func parseGrade(title string) Grade {
	if strings.Contains(title, "高一") || strings.Contains(title, "高1") {
		return GradeOne
	}
	if strings.Contains(title, "高二") || strings.Contains(title, "高2") {
		return GradeTwo
	}
	return ""
}

func parseQuery(raw string) Query {
	normalized := strings.NewReplacer("，", " ", ",", " ", "、", " ", "+", " ").Replace(strings.TrimSpace(raw))
	query := Query{}
	for _, token := range strings.Fields(normalized) {
		if match := classToken.FindStringSubmatch(token); match != nil {
			query.ClassNo = classNumber(token)
			continue
		}
		if grade := parseGrade(token); grade != "" {
			query.Grade = grade
			continue
		}
		query.NameTokens = append(query.NameTokens, normalizeName(token))
	}
	return query
}

func Search(students []Student, raw string, limit, offset int) (SearchResponse, Query) {
	query := parseQuery(raw)
	if strings.TrimSpace(raw) == "" {
		return SearchResponse{Items: []Student{}, Limit: limit, Offset: offset}, query
	}
	matches := make([]Student, 0)
	for _, item := range students {
		ok := true
		for _, token := range query.NameTokens {
			if !strings.Contains(item.NameKey, token) {
				ok = false
				break
			}
		}
		if query.Grade != "" && item.Grade != query.Grade {
			ok = false
		}
		if query.ClassNo > 0 && classNumber(item.ClassName) != query.ClassNo {
			ok = false
		}
		if ok {
			matches = append(matches, item)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		leftScore, rightScore := 0, 0
		for _, token := range query.NameTokens {
			leftScore += nameScore(left.NameKey, token)
			rightScore += nameScore(right.NameKey, token)
		}
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		if left.Grade != right.Grade {
			return left.Grade < right.Grade
		}
		return classNumber(left.ClassName) < classNumber(right.ClassName)
	})
	if offset > len(matches) {
		offset = len(matches)
	}
	end := len(matches)
	if limit < len(matches)-offset {
		end = offset + limit
	}
	items := matches[offset:end]
	return SearchResponse{Items: items, Total: len(matches), Limit: limit, Offset: offset, HasMore: end < len(matches)}, query
}

func nameScore(nameKey, token string) int {
	if nameKey == token {
		return 0
	}
	if strings.HasPrefix(nameKey, token) {
		return 1
	}
	return 2
}

func classNumber(value string) int {
	match := classToken.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0
	}
	if number, err := strconv.Atoi(match[1]); err == nil {
		return number
	}
	return classDigits[match[1]]
}
