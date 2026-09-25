package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
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

// probeInterval 文件指纹探测节流窗口。
// 热重载的真实场景是管理员手工替换名单文件，1 秒延迟无感；
// 收益是消除每请求 3 次 os.Stat 的系统调用开销。
const probeInterval = time.Second

// reloadCooldown 重载失败后的冷却窗口：失败指纹未变时不再重复读盘与解析。
// 运维更新名单采用"临时文件→原子替换"，1 秒探测节流与 2 秒失败冷却是同一条时间线上的两个窗口。
const reloadCooldown = 2 * time.Second

type studentStore struct {
	dir            string
	mu             sync.RWMutex
	reloadMu       sync.Mutex
	items          []Student
	stamps         map[string]fileStamp
	lastProbe      atomic.Int64         // 上次文件指纹探测的 Unix 毫秒（节流基准）
	lastFailStamps map[string]fileStamp // 失败时的文件指纹：文件未变则冷却，变化则立即重试
	lastFailAt     atomic.Int64         // 上次重载失败的 Unix 毫秒（与 lastProbe 共用同一时钟）
	lastFailErr    error                // 上次失败的原始错误：冷却期对外保留根因，不退化为无信息量的哨兵
	now            func() time.Time     // 可注入时钟，供测试确定性推进探测与冷却（默认 time.Now）
}

func loadStudents(dir string) ([]Student, error) {
	students := make([]Student, 0)
	seen := make(map[string]struct{})
	found := false
	for _, grade := range knownGrades {
		path := filepath.Join(dir, string(grade)+".json")
		payload, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			// 年段文件按需加载：目录里存在哪个就加载哪个（高一/高二/高三自由组合）
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取%s数据失败: %w", grade, err)
		}
		found = true
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
				student := newStudent(name, grade, className)
				key := string(grade) + "\x00" + className + "\x00" + student.NameKey
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				students = append(students, student)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("数据文件缺失，请将 %s、%s 或 %s 之一放入数据目录",
			GradeOne+".json", GradeTwo+".json", GradeThree+".json")
	}
	return students, nil
}

func newStudentStore(dir string) (*studentStore, error) {
	store := &studentStore{dir: dir, now: time.Now}
	if err := store.reload(true); err != nil {
		return nil, err
	}
	return store, nil
}

// probeThrottled 判定本次是否应跳过文件指纹探测。
// 首个请求与超过 probeInterval 的请求返回 false（需要探测），其余返回 true。
// 并发安全：使用原子 CAS 保证同一窗口内仅一个请求获得探测权，其余被节流。
func (s *studentStore) probeThrottled() bool {
	nowMS := s.now().UnixMilli()
	last := s.lastProbe.Load()
	if last == 0 || nowMS-last >= probeInterval.Milliseconds() {
		// 需要探测：CAS 占有本次探测权；若失败说明其他请求刚探测过，按节流处理
		if s.lastProbe.CompareAndSwap(last, nowMS) {
			return false
		}
		return true
	}
	return true
}

// errDataUnavailable 表示名单当前不可用：启动时数据目录无有效名单，或已观察到的
// 名单变更在重载时失败。共享哨兵值，避免每次读取为同一原因分配新错误对象。
var errDataUnavailable = errors.New("数据不可用")

// view 返回当前可用的名单只读视图（零拷贝），数据不可用时返回错误。
// 可用性判定完全由 reload 承担：探测、串行重载、失败冷却与恢复都在 reload 内闭环，
// 因此 reload 一旦返回错误，内存中保留的旧快照就不会被当作当前可用数据返回。
// 并发安全性：reload 整体替换 s.items 切片，从不就地修改底层数组，
// 因此持有旧切片的读者不受后续重载影响。
// 调用方必须只读：不得修改返回切片或其元素（需要可写副本时自行拷贝）。
func (s *studentStore) view() ([]Student, error) {
	if err := s.reload(false); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items, nil
}

// reload 探测文件指纹并串行重载，是数据可用性的唯一判定点。
// force 供启动自举使用，绕过探测节流与失败冷却。
// 探测节流只表示"本窗口已有人探测过"，绝不表示"当前数据健康"：
// 命中节流时返回 nil 表示"沿用已发布的当前状态"（成功即返回当前快照，失败即返回不可用）。
// 失败已发布时不参与探测节流：此时节流会让"指纹变化立即重试"失效，
// 使已修好的名单最长延迟一个节流窗口才恢复，多一次 stat 的代价远低于持续不可用。
func (s *studentStore) reload(force bool) error {
	// 探测节流：force（启动自举）与超过窗口的请求才真正探测文件指纹
	if !force && s.lastFailAt.Load() == 0 && s.probeThrottled() {
		return nil
	}
	stamps, err := dataStamps(s.dir)
	if err != nil {
		s.recordFailure(nil, err)
		return err
	}
	s.mu.RLock()
	unchanged := !force && sameStamps(s.stamps, stamps)
	s.mu.RUnlock()
	if unchanged {
		return nil
	}

	// 互斥重载：确保高并发下同一时刻仅有一个协程执行磁盘读取与反序列化，防御惊群
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	// 双重检查：排队获得重载锁的协程在此检测前一个协程是否已经完成重载
	s.mu.RLock()
	unchanged = !force && sameStamps(s.stamps, stamps)
	s.mu.RUnlock()
	if unchanged {
		return nil
	}

	// 失败冷却：数据损坏期间每次请求都重试解析坏文件会放大 IO 与日志。
	// 仅当文件指纹与失败时相同（坏文件未变）才冷却；文件被修复（指纹变化）则立即重试。
	// 冷却期返回原始失败原因而非裸哨兵：运维在 /api/search 日志中仍能区分
	// 解析失败、标题不一致、班级格式异常等具体成因。
	if s.cooling(stamps) {
		return s.coolingError()
	}
	items, err := loadStudents(s.dir)
	if err != nil {
		s.recordFailure(stamps, err)
		return err
	}
	s.mu.Lock()
	s.items = items
	s.stamps = stamps
	s.lastFailStamps = nil
	s.lastFailErr = nil
	s.lastFailAt.Store(0)
	s.mu.Unlock()
	if !force {
		logInfof("data reloaded: %d students", len(items))
	}
	return nil
}

// cooling 判定当前是否处于失败冷却窗口：失败指纹未变且距上次失败不足 reloadCooldown。
// 与 probeThrottled 共用 s.now()，使探测节流与失败冷却可在测试中沿同一条时间线推进。
func (s *studentStore) cooling(stamps map[string]fileStamp) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	last := s.lastFailAt.Load()
	if last == 0 || !sameStamps(s.lastFailStamps, stamps) {
		return false
	}
	return s.now().UnixMilli()-last < reloadCooldown.Milliseconds()
}

// recordFailure 发布失败状态：记录失败指纹、失败时间与原始错误，使后续读取在冷却
// 窗口内继续观察到不可用，而不是把保留在内存中的旧快照误报为当前健康。
// dataStamps 自身失败时 stamps 为 nil，此时只更新时间基准（缺失目录同样需要冷却）。
func (s *studentStore) recordFailure(stamps map[string]fileStamp, cause error) {
	s.mu.Lock()
	s.lastFailStamps = stamps
	s.lastFailErr = cause
	s.lastFailAt.Store(s.now().UnixMilli())
	s.mu.Unlock()
}

// coolingError 返回冷却期内应对外暴露的错误：保留上次失败的原始原因，
// 便于运维从 /api/search 的错误日志直接判断名单损坏的具体成因。
// 没有记录根因时退回共享哨兵，避免每次读取分配新错误对象。
func (s *studentStore) coolingError() error {
	s.mu.RLock()
	cause := s.lastFailErr
	s.mu.RUnlock()
	if cause == nil {
		return errDataUnavailable
	}
	return cause
}

func dataStamps(dir string) (map[string]fileStamp, error) {
	stamps := make(map[string]fileStamp, len(knownGrades))
	for _, grade := range knownGrades {
		path := filepath.Join(dir, string(grade)+".json")
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		stamps[path] = fileStamp{size: info.Size(), modTime: info.ModTime()}
	}
	// 空目录守卫：至少一个年段文件必须存在，避免静默空跑
	if len(stamps) == 0 {
		return nil, fmt.Errorf("数据文件缺失，请将 %s、%s 或 %s 之一放入数据目录",
			GradeOne+".json", GradeTwo+".json", GradeThree+".json")
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
