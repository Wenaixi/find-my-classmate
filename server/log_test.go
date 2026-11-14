package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
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
