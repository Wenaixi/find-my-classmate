package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
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

func TestStoreHotReload(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now
	if got, _ := store.view(); len(got) != 3 {
		t.Fatalf("初始应 3 条，实际 %d", len(got))
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"},{"姓名":"新人"}]}}`), 0o644)
	// 超过探测窗口后应发现变更并重载
	clock.advance(2 * time.Second)
	got, err := store.view()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("热重载后应 4 条，实际 %d", len(got))
	}
}

func TestStoreReloadErrorKeepsOldDataAndRecovers(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	if _, err := store.view(); err == nil {
		t.Fatal("损坏文件 view 应报错")
	}
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`), 0o644)
	got, err := store.view()
	if err != nil || len(got) != 3 {
		t.Fatalf("修复后应恢复 3 条，err=%v len=%d", err, len(got))
	}
}

func TestStoreViewConcurrent(t *testing.T) {
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
				got, err := store.view()
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

func TestStoreConcurrentHotReloadStampede(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now
	if got, _ := store.view(); len(got) != 3 {
		t.Fatalf("初始应 3 条，实际 %d", len(got))
	}

	// 模拟写入新名单，触发指纹变更
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"},{"姓名":"新人"}]}}`), 0o644)

	// 推进超过探测窗口后，单次调用应完成热重载（F72：窗口过后首次探测生效）
	clock.advance(2 * time.Second)
	if got, err := store.view(); err != nil || len(got) != 4 {
		t.Fatalf("窗口过后重载应 4 条，err=%v len=%d", err, len(got))
	}

	// 并发读安全：探测节流窗口内所有请求都应读到一致的新视图（4 条），
	// 不 panic、不撕裂、不出现新旧混合。重载是整体换切片，读者持有旧底层数组也不受影响。
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				got, err := store.view()
				if err != nil {
					t.Errorf("并发快照失败: %v", err)
					return
				}
				if len(got) != 4 {
					t.Errorf("并发快照应恒为 4 条，实际 %d", len(got))
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

func TestRateLimitAutomaticSweep(t *testing.T) {
	clock := &fakeClock{current: time.Unix(0, 0)}
	limiter := newRateLimiter(60, time.Second)
	limiter.now = clock.Now
	_, _ = limiter.allow("1.2.3.4")
	_, _ = limiter.allow("5.6.7.8")

	// 时钟前进 15 分钟，新 IP 发起访问，应自驱动清理超过 10 分钟未活跃的旧桶
	clock.current = clock.current.Add(15 * time.Minute)
	_, _ = limiter.allow("9.9.9.9")

	limiter.mu.Lock()
	_, hasOld1 := limiter.buckets["1.2.3.4"]
	_, hasOld2 := limiter.buckets["5.6.7.8"]
	_, hasNew := limiter.buckets["9.9.9.9"]
	limiter.mu.Unlock()

	if hasOld1 || hasOld2 {
		t.Fatalf("超过空闲时间的旧桶应被自动淘汰，实际仍在: 1.2.3.4=%v, 5.6.7.8=%v", hasOld1, hasOld2)
	}
	if !hasNew {
		t.Fatal("新访问的 IP 桶应正常存在")
	}
}

func TestStoreProbeThrottle(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now
	// 首个请求：lastProbe 为零，应需要探测
	if store.probeThrottled() {
		t.Fatal("首个请求应需要探测，实际被节流")
	}
	// 200ms 后仍处 1 秒窗口内：应被节流
	clock.advance(200 * time.Millisecond)
	if !store.probeThrottled() {
		t.Fatal("节流窗口内应被节流，实际触发了探测")
	}
	// 推进超过 probeInterval：应再次允许探测
	clock.advance(2 * time.Second)
	if store.probeThrottled() {
		t.Fatal("超过探测窗口后应允许探测，实际被节流")
	}
}

// F72：view 返回零拷贝视图——不得分配新切片
func TestStoreViewNoCopy(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.view()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.view()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("视图不应为空")
	}
	// 同一切片底层数组：取首元素地址比较
	if unsafe.SliceData(first) != unsafe.SliceData(second) {
		t.Error("view 应返回同一底层数组（零拷贝），实际发生了拷贝")
	}
}
