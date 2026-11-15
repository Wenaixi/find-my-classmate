package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// FMC_LOG_LEVEL=error 时热重载日志不应输出（裸 log.Printf 绕过级别控制）
func TestReloadLogRespectsLogLevel(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOut)

	oldLevel := logLevel
	logLevel = levelError
	defer func() { logLevel = oldLevel }()

	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte(`{"区县":"福清市","学校":[{"校名":"福清一中","年级":[{"年级名":"高一","班级":[{"班名":"1班","学生":[{"姓名":"王皓轩"},{"姓名":"张三"},{"姓名":"新同学"}]}]}]}]}`), 0o644)
	_, _ = store.view()

	if buf.Len() > 0 {
		t.Fatalf("error 级别下热重载不应输出日志，实际: %s", buf.String())
	}
}

// 日志文件权限应为 0600：日志含数据不可用的根因（名单文件名、班名）与脱敏 IP，
// 多用户宿主机上不应对其他本地用户可读（渗透测试 F5）。
// Windows 无 POSIX 权限位语义（os.OpenFile 的 perm 参数基本被忽略），
// 本用例只在 Linux（CI）验证；Windows 上跳过并在 CI 兜底。
func TestServerLogFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 无 POSIX 权限位语义；由 CI（Linux）验证")
	}
	dir := t.TempDir()
	_, file, err := openLog(dir)
	if err != nil {
		t.Fatalf("openLog 失败: %v", err)
	}
	_ = file.Close()
	info, err := os.Stat(filepath.Join(dir, "log", "server.log"))
	if err != nil {
		t.Fatalf("stat 日志文件失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("日志文件权限 = %#o，期望 0600", perm)
	}
}
