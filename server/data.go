package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	dir            string
	mu             sync.RWMutex
	items          []Student
	stamps         map[string]fileStamp
	lastFailStamps map[string]fileStamp // 失败时的文件指纹：文件未变则冷却，变化则立即重试
	lastFailAt     time.Time
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
	// 失败冷却：数据损坏期间每次请求都重试解析坏文件会放大 IO 与日志。
	// 仅当文件指纹与失败时相同（坏文件未变）才冷却 2 秒；文件被修复（指纹变化）则立即重试。
	s.mu.RLock()
	cooling := !force && sameStamps(s.lastFailStamps, stamps) && time.Since(s.lastFailAt) < 2*time.Second
	s.mu.RUnlock()
	if cooling {
		return errors.New("数据重载失败冷却中（上次尝试 2 秒内）")
	}
	items, err := loadStudents(s.dir)
	if err != nil {
		s.mu.Lock()
		s.lastFailStamps = stamps
		s.lastFailAt = time.Now()
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.items = items
	s.stamps = stamps
	s.lastFailStamps = nil
	s.lastFailAt = time.Time{}
	s.mu.Unlock()
	if !force {
		logInfof("data reloaded: %d students", len(items))
	}
	return nil
}

func dataStamps(dir string) (map[string]fileStamp, error) {
	stamps := make(map[string]fileStamp, 2)
	for _, grade := range []Grade{GradeOne, GradeTwo} {
		path := filepath.Join(dir, string(grade)+".json")
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("数据文件缺失，请将 %s 放入数据目录: %w", string(grade)+".json", err)
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