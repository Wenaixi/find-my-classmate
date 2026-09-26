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
	dir                 string
	mu                  sync.RWMutex
	reloadMu            sync.Mutex
	items               []Student
	stamps              map[string]fileStamp
	lastProbe           atomic.Int64         // 上次文件指纹探测的 Unix 毫秒（节流基准）
	lastFailAt          atomic.Int64         // 上次重载失败的 Unix 毫秒（与 lastProbe 共用同一时钟）
	lastFailStamps      map[string]fileStamp // 失败时的文件指纹：文件未变则冷却，变化则立即重试
	lastFailStampKnown  bool                  // 失败是否发生在指纹采集阶段（dataStamps 失败时为 false）：
	// 显式区分"失败且指纹未知"与"失败且指纹已知"，避免 lastFailStamps 为 nil
	// 同时表示"目录缺失"与"空指纹"两种情形导致冷却判定失效。
	lastFailErr         error     // 上次失败的原始错误：冷却期对外保留根因，不退化为无信息量的哨兵
	now                 func() time.Time // 构造注入的时钟，供测试确定性推进探测与冷却
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
		classNos := make(map[string]ClassParseResult, len(document.Roster))
		for className := range document.Roster {
			// C4：解析一次、三处消费（校验/排序/派生），不再对同一类名重复正则+Atoi。
			// C3：三态校验——旧实现只检 classNumber==0，溢出（-1）静默放行并流入排序
			parsed := parseClassName(className)
			if !parsed.Valid {
				return nil, errors.New("班级格式异常")
			}
			classNos[className] = parsed
			classes = append(classes, className)
		}
		sort.Slice(classes, func(i, j int) bool { return classNos[classes[i]].ClassNo < classNos[classes[j]].ClassNo })
		for _, className := range classes {
			for _, item := range document.Roster[className] {
				name := item.Name
				if name == "" {
					return nil, errors.New("学生记录格式异常")
				}
				student := newStudent(name, grade, className, classNos[className])
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

// newStudentStore 构造并立即完成首次加载（force 路径）。
// now 由构造函数注入：探测节流与失败冷却共用同一条时间线，
// 测试通过构造参数推进时间，不从外部改写可写字段。
func newStudentStore(dir string, now func() time.Time) (*studentStore, error) {
	store := &studentStore{dir: dir, now: now}
	if err := store.reload(true); err != nil {
		return nil, err
	}
	return store, nil
}

// recoveryDue 是自恢复时机的单一判定点：探测节流（1 秒窗口）与失败冷却
// （2 秒窗口）两条时间轴在此汇合，由调用方传入统一取样的 now。
// 「同一条时间线」因此从注释承诺变成代码事实——改任一窗口不会绕开另一条轴。
//
// probe=true 表示应探测文件指纹（节流窗口已过，或尚无探测基准）。
// retry=true 表示可重试失败重载（冷却窗口已过，或尚无失败基准）。
// 失败期间的指纹变化重试由 cooling 单独判定，不在本函数职责内。
func (s *studentStore) recoveryDue(now time.Time) (probe bool, retry bool) {
	nowMS := now.UnixMilli()
	lastProbe := s.lastProbe.Load()
	probe = lastProbe == 0 || nowMS-lastProbe >= probeInterval.Milliseconds()
	lastFail := s.lastFailAt.Load()
	retry = lastFail == 0 || nowMS-lastFail >= reloadCooldown.Milliseconds()
	return probe, retry
}

// probeThrottled 判定本次是否应跳过文件指纹探测。
// 窗口是否已过由 recoveryDue 单点判定；此处只负责并发安全：
// 用原子 CAS 保证同一窗口内仅一个请求获得探测权，其余被节流。
func (s *studentStore) probeThrottled() bool {
	now := s.now()
	if probe, _ := s.recoveryDue(now); !probe {
		return true
	}
	// 需要探测：CAS 占有本次探测权；若失败说明其他请求刚探测过，按节流处理
	if s.lastProbe.CompareAndSwap(s.lastProbe.Load(), now.UnixMilli()) {
		return false
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

// Size 返回当前名单的学生数，供启动自举日志使用。
// 只读计数入口：不暴露切片本身，维持"view() 是唯一数据访问入口"的纪律。
func (s *studentStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
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
	s.lastFailStampKnown = false
	s.lastFailErr = nil
	s.lastFailAt.Store(0)
	s.mu.Unlock()
	if !force {
		logInfof("data reloaded: %d students", len(items))
	}
	return nil
}

// cooling 判定当前是否处于失败冷却窗口：距上次失败不足 reloadCooldown，
// 且失败指纹未变（文件被修复则立即重试）。
// 指纹采集阶段失败（lastFailStampKnown 为 false）时没有可比对的指纹，
// 按"未变化"处理：仅由冷却窗口约束重试频率。修复前 lastFailStamps 为 nil 时
// sameStamps(nil, stamps) 恒为 false，冷却判定永不成立，与注释承诺相反。
func (s *studentStore) cooling(stamps map[string]fileStamp) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastFailStampKnown && !sameStamps(s.lastFailStamps, stamps) {
		return false
	}
	_, retry := s.recoveryDue(s.now())
	return !retry
}

// recordFailure 发布失败状态：记录失败指纹、失败时间与原始错误，使后续读取在冷却
// 窗口内继续观察到不可用，而不是把保留在内存中的旧快照误报为当前健康。
// stamps 为 nil 表示失败发生在指纹采集阶段（dataStamps 自身失败，如目录不可读），
// 此时 lastFailStampKnown 为 false，冷却判定不再依赖指纹比对。
func (s *studentStore) recordFailure(stamps map[string]fileStamp, cause error) {
	s.mu.Lock()
	s.lastFailStamps = stamps
	s.lastFailStampKnown = stamps != nil
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
