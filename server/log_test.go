package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// F29：FMC_LOG_LEVEL=error 时热重载日志不应输出（裸 log.Printf 绕过级别控制）
func TestReloadLogRespectsLogLevel(t *testing.T) {
	dir := t.TempDir()
	writeTestFiles(t, dir)
	store, err := newStudentStore(dir)
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

	_ = os.WriteFile(filepath.Join(dir, "高一.json"), []byte("{\"标题\":\"福清一中2025级高一编班名单\",\"名单\":{\"1班\":[{\"姓名\":\"王皓轩\"},{\"姓名\":\"张三\"},{\"姓名\":\"新同学\"}]}}"), 0o644)
	_, _ = store.snapshot()

	if buf.Len() > 0 {
		t.Fatalf("error 级别下热重载不应输出日志，实际: %s", buf.String())
	}
}
