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

// newTestStore 构造指向临时目录的 studentStore，是数据层 fixture 的唯一入口。
// 供 API、请求链与版本测试复用：学生数据的构造与断言归属数据模块，
// 各调用方只提供自己关心的名单文件。
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

// 重载失败后 fail-closed：失败一经发布，内存中保留的旧快照不再对外服务。
// 修复后指纹变化必须立即重试并恢复，不能被探测节流或失败冷却挡住。
func TestStoreReloadFailureIsFailClosedThenRecovers(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now

	// 破坏名单：应发布失败并进入不可用
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	clock.advance(2 * time.Second)
	if _, err := store.view(); err == nil {
		t.Fatal("损坏文件 view 应报错")
	}

	// 不推进时钟：仍在探测节流窗口内，reload 会跳过探测并返回 nil。
	// 此时只有 fail-closed 检查能阻止旧快照对外服务——若该检查缺失，本断言必须失败。
	if got, err := store.view(); err == nil || len(got) != 0 {
		t.Fatalf("探测窗口内失败后仍应不可用，不得返回旧快照，实际 err=%v len=%d", err, len(got))
	}

	// 修复文件：指纹变化后必须立即重试并恢复，无需等待冷却或探测窗口
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`), 0o644)
	got, err := store.view()
	if err != nil || len(got) != 3 {
		t.Fatalf("修复后应立即恢复 3 条，err=%v len=%d", err, len(got))
	}

	// 恢复后再推进一个探测窗口，数据仍应稳定可用
	clock.advance(2 * time.Second)
	if got, err := store.view(); err != nil || len(got) != 3 {
		t.Fatalf("恢复后应持续可用 3 条，err=%v len=%d", err, len(got))
	}
}

// 冷却期内对外暴露的错误必须保留原始失败原因：运维在 /api/search 的
// error 日志中要能区分解析失败、标题不一致、班级格式异常等具体成因，
// 而不是只看到无信息量的"数据不可用"。
func TestStoreCoolingPreservesFailureCause(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now

	// 破坏名单，首次失败会带上解析根因
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	clock.advance(2 * time.Second)
	first, err := store.view()
	if err == nil {
		t.Fatal("损坏文件 view 应报错")
	}
	if len(first) != 0 {
		t.Fatalf("fail-closed 下不应返回旧快照，实际 %d 条", len(first))
	}
	firstMsg := err.Error()
	if !strings.Contains(firstMsg, "解析") {
		t.Errorf("首次失败错误应说明解析失败，实际 %q", firstMsg)
	}

	// 冷却窗口内：文件未变，仍处于冷却，应保留同一根因
	cooling, err := store.view()
	if err == nil {
		t.Fatal("冷却期内 view 应继续报错")
	}
	if len(cooling) != 0 {
		t.Fatalf("冷却期内不应返回旧快照，实际 %d 条", len(cooling))
	}
	if coolingMsg := err.Error(); coolingMsg != firstMsg {
		t.Errorf("冷却期错误应保留原始根因\n首次: %q\n冷却: %q", firstMsg, coolingMsg)
	}
}

// 失败已发布时探测节流必须让位：否则"指纹变化立即重试"失效，
// 已修好的名单会最长延迟一个节流窗口才恢复。
func TestStoreRecoveryBypassesProbeThrottle(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{current: time.Now()}
	store.now = clock.Now

	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	clock.advance(2 * time.Second)
	if _, err := store.view(); err == nil {
		t.Fatal("损坏文件 view 应报错")
	}

	// 未推进时钟：仍在探测节流窗口内。修复后必须仍能立即恢复。
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"王皓轩"},{"姓名":"张三"}]}}`), 0o644)
	if got, err := store.view(); err != nil || len(got) != 3 {
		t.Fatalf("探测窗口内修复也应立即恢复 3 条，err=%v len=%d", err, len(got))
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
					t.Errorf("并发视图应恒为 3 条，实际 %d", len(got))
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

	// 推进超过探测窗口后，单次调用应完成热重载（窗口过后首次探测生效）
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
					t.Errorf("并发视图失败: %v", err)
					return
				}
				if len(got) != 4 {
					t.Errorf("并发视图应恒为 4 条，实际 %d", len(got))
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

// view 返回零拷贝视图——不得分配新切片
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

// Step B：Size 是只读视图之外的启动期计数入口——main.go 自举日志用它，
// 不再直接读 store.items 字段（"view 是唯一读入口"纪律的补充通道）。
func TestStoreSize(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.view()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Size(); got != len(items) {
		t.Errorf("Size() = %d，期望 %d（与 view 长度一致）", got, len(items))
	}
}
