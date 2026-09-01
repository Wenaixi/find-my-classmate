package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeTestFiles(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"高一.json": `{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"},{"姓名":"张 三"}]}}`,
		"高二.json": `{"标题":"福清一中2025级高二编班名单","名单":{"2班":[{"姓名":"李四"}]}}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadStudentsDedup(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(students) != 3 {
		t.Fatalf("应加载 3 条（王皓轩、张三、李四），实际 %d", len(students))
	}
	for _, s := range students {
		if s.Name == "张 三" {
			t.Error("张 三 不应被加载（与张三归一化后重复）")
		}
	}
}

func TestLoadStudentsBOMStripped(t *testing.T) {
	dir := t.TempDir()
	content := []byte{0xEF, 0xBB, 0xBF}
	content = append(content, []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"BOM同学"}]}}`)...)
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), content, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(`{"标题":"福清一中2025级高二编班名单","名单":{"1班":[{"姓名":"乙"}]}}`), 0o644)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("带 BOM 文件应可加载: %v", err)
	}
	if len(students) != 2 {
		t.Fatalf("应加载 2 条，实际 %d", len(students))
	}
}

func TestLoadStudentsTitleMismatch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高二编班名单","名单":{"1班":[{"姓名":"甲"}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(`{"标题":"福清一中2025级高二编班名单","名单":{"1班":[{"姓名":"乙"}]}}`), 0o644)
	if _, err := loadStudents(dir); err == nil || !strings.Contains(err.Error(), "不一致") {
		t.Fatalf("标题与文件名不一致应报错，实际 %v", err)
	}
}

func TestLoadStudentsBadClassKey(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"X班":[{"姓名":"甲"}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "高二.json"), []byte(`{"标题":"福清一中2025级高二编班名单","名单":{"1班":[{"姓名":"乙"}]}}`), 0o644)
	if _, err := loadStudents(dir); err == nil || !strings.Contains(err.Error(), "班级格式异常") {
		t.Fatalf("非法班级键应报错，实际 %v", err)
	}
}

func TestSnapshotHotReload(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := store.snapshot(); len(got) != 3 {
		t.Fatalf("初始应 3 条，实际 %d", len(got))
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"},{"姓名":"新人"}]}}`), 0o644)
	got, err := store.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("热重载后应 4 条，实际 %d", len(got))
	}
}

func TestSnapshotErrorKeepsOldDataAndRecovers(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	if _, err := store.snapshot(); err == nil {
		t.Fatal("损坏文件 snapshot 应报错")
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`), 0o644)
	got, err := store.snapshot()
	if err != nil || len(got) != 3 {
		t.Fatalf("修复后应恢复 3 条，err=%v len=%d", err, len(got))
	}
}

func TestSnapshotConcurrent(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				got, err := store.snapshot()
				if err != nil {
					t.Error(err)
					return
				}
				if len(got) != 3 {
					t.Errorf("并发快照应恒为 3 条，实际 %d", len(got))
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestRateLimitSweep(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(60, time.Second)
	limiter.now = clock.Now
	_, _ = limiter.allow("1.2.3.4")
	_, _ = limiter.allow("5.6.7.8")
	limiter.sweep(clock.Now().Add(25*time.Hour), 24*time.Hour)
	limiter.mu.Lock()
	n := len(limiter.buckets)
	limiter.mu.Unlock()
	if n != 0 {
		t.Fatalf("空闲桶应被清理，剩余 %d", n)
	}
}

func TestSnapshotReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, _ := newStudentStore(dir)
	got, _ := store.snapshot()
	got[0].Name = "篡改"
	fresh, _ := store.snapshot()
	if fresh[0].Name == "篡改" {
		t.Fatal("snapshot 应返回副本，修改不应影响 store")
	}
}
