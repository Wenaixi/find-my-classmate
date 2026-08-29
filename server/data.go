package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type sourceStudent struct {
	Name string `json:"姓名"`
}

type sourceDocument struct {
	Title  string                     `json:"标题"`
	Roster map[string][]sourceStudent `json:"名单"`
}

type fileStamp struct {
	size    int64
	modTime time.Time
}

type studentStore struct {
	dir    string
	mu     sync.RWMutex
	items  []Student
	stamps map[string]fileStamp
}

func loadStudents(dir string) ([]Student, error) {
	students := make([]Student, 0)
	seen := make(map[string]struct{})
	for _, grade := range []Grade{GradeOne, GradeTwo} {
		path := filepath.Join(dir, string(grade)+".json")
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取%s数据失败: %w", grade, err)
		}
		payload = bytes.TrimPrefix(payload, []byte{0xEF, 0xBB, 0xBF})
		var document sourceDocument
		if err := json.Unmarshal(payload, &document); err != nil {
			return nil, fmt.Errorf("解析%s数据失败: %w", grade, err)
		}
		if document.Title == "" || parseGrade(document.Title) != grade {
			return nil, errors.New("文件名与年级标题不一致")
		}
		if document.Roster == nil {
			return nil, errors.New("名单结构异常")
		}
		classes := make([]string, 0, len(document.Roster))
		for className := range document.Roster {
			if classNumber(className) == 0 {
				return nil, errors.New("班级格式异常")
			}
			classes = append(classes, className)
		}
		sort.Slice(classes, func(i, j int) bool { return classNumber(classes[i]) < classNumber(classes[j]) })
		for _, className := range classes {
			for _, item := range document.Roster[className] {
				name := item.Name
				if name == "" {
					return nil, errors.New("学生记录格式异常")
				}
				key := string(grade) + "\x00" + className + "\x00" + normalizeName(name)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				students = append(students, Student{Name: name, NameKey: normalizeName(name), Grade: grade, ClassName: className})
			}
		}
	}
	return students, nil
}

func newStudentStore(dir string) (*studentStore, error) {
	store := &studentStore{dir: dir}
	if err := store.reload(true); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *studentStore) snapshot() ([]Student, error) {
	if err := s.reload(false); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Student, len(s.items))
	copy(items, s.items)
	return items, nil
}

func (s *studentStore) reload(force bool) error {
	stamps, err := dataStamps(s.dir)
	if err != nil {
		return err
	}
	s.mu.RLock()
	unchanged := !force && sameStamps(s.stamps, stamps)
	s.mu.RUnlock()
	if unchanged {
		return nil
	}
	items, err := loadStudents(s.dir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.items = items
	s.stamps = stamps
	s.mu.Unlock()
	if !force {
		log.Printf("data reloaded: %d students", len(items))
	}
	return nil
}

func dataStamps(dir string) (map[string]fileStamp, error) {
	stamps := make(map[string]fileStamp, 2)
	for _, grade := range []Grade{GradeOne, GradeTwo} {
		path := filepath.Join(dir, string(grade)+".json")
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("读取数据文件状态失败: %w", err)
		}
		stamps[path] = fileStamp{size: info.Size(), modTime: info.ModTime()}
	}
	return stamps, nil
}

func sameStamps(left, right map[string]fileStamp) bool {
	if len(left) != len(right) {
		return false
	}
	for path, stamp := range right {
		if left[path] != stamp {
			return false
		}
	}
	return true
}
