package services

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D21 下载入库契约 + 解压剥离单一顶层目录
// ─────────────────────────────────────────────────────────────

func TestStripSingleTopDir(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  string
	}{
		{"全部同顶层", []string{"Title/1.jpg", "Title/2.jpg"}, "Title"},
		{"根目录直接有文件", []string{"1.jpg", "Title/2.jpg"}, ""},
		{"多个顶层", []string{"A/1.jpg", "B/2.jpg"}, ""},
		{"目录条目忽略", []string{"Title/", "Title/1.jpg"}, "Title"},
		{"反斜杠归一", []string{`Title\1.jpg`, `Title\2.jpg`}, "Title"},
		{"空列表", nil, ""},
		{"嵌套目录仍在同一顶层", []string{"Title/sub/1.jpg", "Title/2.jpg"}, "Title"},
	}
	for _, c := range cases {
		if got := stripSingleTopDir(c.names); got != c.want {
			t.Errorf("%s: stripSingleTopDir=%q，期望 %q", c.name, got, c.want)
		}
	}
}

func TestExtractStripsSingleTopFolder(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "x.zip")
	mustWriteZipContent(t, zipPath, map[string]string{
		"Title/1.jpg": "a",
		"Title/2.jpg": "b",
		"Title/metadata": `{"gid":"1"}`,
	})
	extractDir := filepath.Join(dir, "out")

	n, err := extractArchive(nil, zipPath, extractDir, false, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if n != 3 {
		t.Errorf("应解压 3 个文件，实际 %d", n)
	}
	// 顶层目录被剥离：图片直接平铺在 extractDir
	for _, name := range []string{"1.jpg", "2.jpg", "metadata"} {
		if _, err := os.Stat(filepath.Join(extractDir, name)); err != nil {
			t.Errorf("剥离后应存在 %q: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(extractDir, "Title")); !os.IsNotExist(err) {
		t.Errorf("顶层目录 Title 不应保留")
	}
}

func TestExtractKeepsFlatStructure(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "x.zip")
	mustWriteZipContent(t, zipPath, map[string]string{
		"1.jpg": "a",
		"2.jpg": "b",
	})
	extractDir := filepath.Join(dir, "out")

	if _, err := extractArchive(nil, zipPath, extractDir, false, nil, nil, nil, nil); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	for _, name := range []string{"1.jpg", "2.jpg"} {
		if _, err := os.Stat(filepath.Join(extractDir, name)); err != nil {
			t.Errorf("平铺结构应保持 %q: %v", name, err)
		}
	}
}

func TestIngestDownloadDir(t *testing.T) {
	db := setupScanPolicyDB(t)
	_ = db

	// 成功：目录内 1 张图 → 单本根模式入库 1 本
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "1.jpg"), "x")
	found, err := ingestDownloadDir(dir)
	if err != nil {
		t.Fatalf("入库应成功: %v", err)
	}
	if found != 1 {
		t.Errorf("应入库 1 本，实际 %d", found)
	}

	// 失败：目录不存在
	if _, err := ingestDownloadDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Errorf("不存在的目录应返回错误")
	}

	// 失败：空目录（0 本）
	emptyDir := t.TempDir()
	if _, err := ingestDownloadDir(emptyDir); err == nil {
		t.Errorf("未发现任何本子时应返回错误")
	}

	// 失败：空路径
	if _, err := ingestDownloadDir(""); err == nil {
		t.Errorf("空路径应返回错误")
	}
}
