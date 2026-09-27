package main

import (
	"bytes"
	"errors"
	"log"
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
// 供不需要修改源文件的测试复用：学生数据的构造与断言归属数据模块，各调用方只提供自己关心的名单文件。
func newTestStore(t *testing.T, files map[string]string) *studentStore {
	t.Helper()
	store, _ := newTestStoreWithDir(t, files, time.Now)
	return store
}

// newTestStoreWithDir 在需要模拟文件变化的测试中同时返回 fixture 目录。
// 目录是测试场景的显式事实，调用方不应从 studentStore 私有字段反向取出它。
func newTestStoreWithDir(t *testing.T, files map[string]string, now func() time.Time) (*studentStore, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store, err := newStudentStore(dir, now)
	if err != nil {
		t.Fatalf("newStudentStore: %v", err)
	}
	return store, dir
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
// TestLoadStudentsDedupAcrossClassNotations 验证「同一人」的判定落在班号上，
// 而不是班名的书写形态上：「1班」与「一班」解析出同一个班号，是同一个班。
//
// 两种写法混用时去重键若用原始班名，同一个人会被加载两次并在响应里出现两条
// 班名不同的记录。断言只锁条数与「班号唯一」，不锁幸存的班名——Go map 的迭代
// 序随机，幸存者是哪一个写法不可确定（实测 200 次分布约 179:21），
// 把它写进断言会造出偶发失败的测试。
func TestLoadStudentsDedupAcrossClassNotations(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"标题":"福清一中2025级高一编班名单","名单":{"1班":[{"姓名":"张三"}],"一班":[{"姓名":"张三"}]}}`), 0o644)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("混用班级写法应可加载: %v", err)
	}
	if len(students) != 1 {
		t.Fatalf("两种写法指向同一班号的同一人，应只加载 1 条，实际 %d 条", len(students))
	}
	if students[0].ClassNo != 1 {
		t.Errorf("班号 = %d，期望 1", students[0].ClassNo)
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
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
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
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

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
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

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

// 数据不可用的根因必须留痕，且只在故障状态发生变化时留一次。
//
// 该测试的存在理由：修复前 /api/health 用 "_" 丢弃 view 的错误、不产出任何日志，
// 而 recordFailure 只更新状态不记录。配合 compose 的 healthcheck（每 30 秒一次）
// 与零用户流量，表现为名单损坏、站点持续 503 degraded、运维侧日志字节数为 0。
// 该场景是实测复现的，不是推理。
//
// 两个方向都断言，缺一不可：
//   - 只断言"有记录"：逐次记录的退化实现同样通过，日志会被每天四万余条噪音淹没；
//   - 只断言"不刷屏"：删掉记录实现后探针数为 0，日志为空，断言同样通过。
func TestFailureCauseIsLoggedOncePerOutage(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOut)

	// 破坏名单，随后只有探针流量（无任何用户查询）
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{broken"), 0o644)
	const probes = 5
	for i := range probes {
		clock.advance(30 * time.Second)
		if _, err := store.view(); err == nil {
			t.Fatalf("探针 #%d：损坏名单 view 应报错", i+1)
		}
	}

	logged := buf.String()
	if !strings.Contains(logged, "解析") {
		t.Errorf("数据不可用必须留下含根因的日志，实际日志 %q", logged)
	}
	if count := strings.Count(logged, "\n"); count != 1 {
		t.Errorf("同一次故障的 %d 次探针应只记录 1 条，实际 %d 条：%q", probes, count, logged)
	}

	// 恢复后再次损坏属于新的一次故障，必须重新留痕。
	// 若实现按「曾经失败过就不再记录」去重，本断言会失败。
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(validGradeOne), 0o644)
	clock.advance(3 * time.Second)
	if _, err := store.view(); err != nil {
		t.Fatalf("修复后应恢复可用，实际 %v", err)
	}
	buf.Reset()
	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{"+"broken"), 0o644)
	clock.advance(3 * time.Second)
	if _, err := store.view(); err == nil {
		t.Fatal("再次损坏后 view 应报错")
	}
	if !strings.Contains(buf.String(), "解析") {
		t.Errorf("故障往复后应重新留痕，实际日志 %q", buf.String())
	}
}

// 失败已发布时探测节流必须让位：否则"指纹变化立即重试"失效，
// 已修好的名单会最长延迟一个节流窗口才恢复。
func TestStoreRecoveryBypassesProbeThrottle(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

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
	store, err := newStudentStore(dir, time.Now)
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
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
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

// 按数据目录实际文件探测年段，缺失的年级文件跳过不报错
func TestLoadStudentsSkipMissingGrade(t *testing.T) {
	dir := t.TempDir()
	// 只放高三，高一高二不存在
	_ = os.WriteFile(filepath.Join(dir, "高三.json"), []byte(`{"标题":"福清一中2025级高三编班名单","名单":{"3班":[{"姓名":"高三甲"}]}}`), 0o644)
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("仅高三存在应成功加载: %v", err)
	}
	if len(students) != 1 {
		t.Fatalf("应加载 1 条，实际 %d", len(students))
	}
	if students[0].Grade != GradeThree {
		t.Errorf("Grade = %q，期望 高三", students[0].Grade)
	}
}

// 空目录应给出明确指引，不静默空跑
func TestLoadStudentsEmptyDirFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadStudents(dir); err == nil {
		t.Fatal("空目录应报错，不应静默空跑")
	}
}

func TestStoreProbeThrottle(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
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
	store, err := newStudentStore(dir, time.Now)
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
	store, err := newStudentStore(dir, time.Now)
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

// 自恢复时机是探测节流与失败冷却两条时间轴的单一判定点：
// 两条窗口共用同一条时间线推进，调用方不再各自取样时钟。
func TestStoreRecoveryDueSingleTimeline(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	// recoveryDue 是纯判定：只回答"此刻是否该探测"，不推进任何状态
	// （推进 lastProbe 是 probeThrottled 的 CAS 职责）。
	// 因此未探测过（lastProbe 为零）时，无论调用多少次都判定为"需探测"。
	probe, _ := store.recoveryDue(clock.Now())
	if !probe {
		t.Error("未探测过时判定应为需要探测")
	}
	// 通过 probeThrottled 推进探测基准：首个请求应获得探测权
	if store.probeThrottled() {
		t.Error("首个请求应获得探测权（不被节流）")
	}
	// 500ms 后仍在 1 秒窗口内：应被节流
	clock.advance(500 * time.Millisecond)
	if !store.probeThrottled() {
		t.Error("500ms 内应被探测节流")
	}
	// 推进超过 1 秒窗口：应恢复探测
	clock.advance(600 * time.Millisecond)
	if store.probeThrottled() {
		t.Error("超过 1s 窗口应恢复探测权")
	}
}

// 目录缺失（dataStamps 自身失败）时同样进入冷却：
// 否则每个请求都重试读盘并写错误日志，冷却机制形同虚设。
// 修复前 lastFailStamps 为 nil，sameStamps(nil, ...) 恒为 false，
// 冷却判定因此永不成立，注释承诺与实现相反。
func TestStoreMissingDirectoryEntersCooldown(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	// 删除整个数据目录，使 dataStamps 的 os.Stat 失败（非 fs.ErrNotExist 之外的路径）
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := store.view(); err == nil {
		t.Fatal("目录移除后 view 应报不可用")
	}

	// 冷却期内再次读取：应观察到不可用，但不应反复读盘重试——
	// 判定入口是 lastFailAt 是否落在冷却窗口内
	clock.advance(100 * time.Millisecond)
	if _, retry := store.recoveryDue(clock.Now()); retry {
		t.Error("100ms 内仍在 2s 冷却窗口，retry 应为 false")
	}
	// 冷却窗口过后：应允许重试
	clock.advance(2 * time.Second)
	if _, retry := store.recoveryDue(clock.Now()); !retry {
		t.Error("超过 2s 冷却窗口后应允许重试")
	}
}

// 指纹采集阶段失败时 cooling 必须成立：
// 修复前 lastFailStamps 为 nil，sameStamps(nil, stamps) 恒为 false，
// cooling 立即返回 false——目录缺失时每个请求都重试读盘并写错误日志，
// 与 data.go 注释"缺失目录同样需要冷却"的承诺相反。
func TestCoolingHoldsWhenStampsUnknown(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	clock := &fakeClock{current: time.Now()}
	store, err := newStudentStore(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	// 模拟指纹采集失败：stamps 为 nil 表示"没有可比对的指纹"
	store.recordFailure(nil, errors.New("数据文件缺失"))
	// cooling 返回 true 表示"当前处于冷却窗口内"
	if !store.cooling(map[string]fileStamp{"x": {size: 1}}) {
		t.Error("刚失败后应处于冷却窗口内（cooling 应为 true）")
	}

	// 冷却窗口过后：允许重试
	clock.advance(3 * time.Second)
	if store.cooling(map[string]fileStamp{"x": {size: 1}}) {
		t.Error("超过冷却窗口后应允许重试（cooling 应为 false）")
	}
}

// TestErrNoRosterMessage 锁定「数据目录无任何年段文件」这条运维指引。
//
// 该文案此前在 loadStudents 与 dataStamps 两处逐字复制；收敛为 errNoRoster
// 单点时首版实现把「或」交给 strings.Join 处理，产出「高二.json、 或 高三.json」，
// 比原句多一个顿号——运维看到的是一条格式错乱的指引。本测试锁住逐字不变。
func TestErrNoRosterMessage(t *testing.T) {
	want := "数据文件缺失，请将 高一.json、高二.json 或 高三.json 之一放入数据目录"
	if got := errNoRoster().Error(); got != want {
		t.Fatalf("运维指引文案漂移\n实际 = %q\n期望 = %q", got, want)
	}
}

// TestErrNoRosterFollowsKnownGrades 验证指引随 knownGrades 自动扩展。
//
// 这是把文案从硬编码改为生成的真正理由：扩展年段只需在 knownGrades 追加，
// 运维提示自动跟随，不会继续提示放置一个已不被支持的文件名。
func TestErrNoRosterFollowsKnownGrades(t *testing.T) {
	original := knownGrades
	t.Cleanup(func() { knownGrades = original })

	knownGrades = []Grade{GradeOne, GradeTwo, GradeThree, "高四"}
	got := errNoRoster().Error()
	if !strings.Contains(got, "高四.json") {
		t.Fatalf("新增年段后指引应包含 高四.json，实际 = %q", got)
	}
	if !strings.HasSuffix(got, "高三.json 或 高四.json 之一放入数据目录") {
		t.Fatalf("末两项之间应为「或」，实际 = %q", got)
	}

	// 单个年段时不应出现悬空的「或」
	knownGrades = []Grade{GradeOne}
	if single := errNoRoster().Error(); !strings.Contains(single, "请将 高一.json 之一") {
		t.Fatalf("单年段时不应有多余连接符，实际 = %q", single)
	}
	// 空目录也不应 panic
	knownGrades = nil
	_ = errNoRoster()
}

// realRosterDir 返回待校验的真实名单目录，ok=false 表示该处没有名单文件。
//
// 默认取仓库的 data/ 目录（校内部署放置名单后的工作位置）；
// FMC_TEST_ROSTER_DIR 可指定其他目录，便于对候选名单先行校验再投放。
func realRosterDir() (dir string, ok bool) {
	dir = os.Getenv("FMC_TEST_ROSTER_DIR")
	if dir == "" {
		dir = filepath.Join("..", "data")
	}
	for _, grade := range knownGrades {
		if _, err := os.Stat(filepath.Join(dir, string(grade)+".json")); err == nil {
			return dir, true
		}
	}
	return dir, false
}

// TestRealRosterIsLoadable 真实名单形态门禁。
//
// 「真实名单不进 git」的决定正确，但它有个此前无人兑现的代价：
// 全部既有测试都用 t.TempDir() 造合成 fixture，CI 永远验不到真实数据的形态。
// 而 loadStudents 对形态漂移是 fail-closed 的——标题不含年段词、
// 或任一姓名为空串，都会让整份文件被拒绝，服务随之 503 整站不可用，
// 而 CI 全绿。ci.yml 的 data job 曾把这一点写成「数据契约由
// server/data_test.go 保证」，那句话当时并不成立。
//
// 探针实测的接受面（2026-09-27）：班名 01班 / 无「班」字、姓名前后带空格、
// 同班重名、跨班同名、新增未知字段均被接受，容错良好；
// 标题不含年段词与空姓名则整份拒绝。
//
// 本用例把这条边界变成可执行的门禁：部署方放置名单后跑一次 go test ./...
// 即可确认形态被接受，而不必等到服务起不来才发现。
// CI 环境无名单文件时跳过——真实名单本就不得进仓库。
func TestRealRosterIsLoadable(t *testing.T) {
	dir, ok := realRosterDir()
	if !ok {
		t.Skip("该目录未放置名单文件（CI 环境恒如此）；校内部署放置 data/*.json 后本校验生效")
	}
	students, err := loadStudents(dir)
	if err != nil {
		t.Fatalf("真实名单被拒绝加载：%v\n"+
			"这会让服务启动后 /api/health 返回 503 degraded、/api/search 返回 500。\n"+
			"请核对名单形态：标题须含年段词（如「...高一编班名单」），姓名不得为空串。", err)
	}
	if len(students) == 0 {
		t.Fatal("真实名单加载成功但学生数为 0")
	}
	// 每名学生都必须带正班号：ClassNo 非正会让「按班级筛选」永远筛不出人。
	// loadStudents 声称已校验 parsed.Valid，此处复核该承诺在真实数据上兑现。
	zeroClass := 0
	for _, s := range students {
		if s.ClassNo <= 0 {
			zeroClass++
		}
	}
	if zeroClass > 0 {
		t.Errorf("真实名单中有 %d/%d 名学生的班号非正，按班级筛选将失效", zeroClass, len(students))
	}
	t.Logf("真实名单校验通过：%d 名学生", len(students))
}
