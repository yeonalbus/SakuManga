package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────
// Round53 阶段三：D13 侧车元数据优先级统一
//
// 优先级（高 → 低）：ComicInfo.xml > metadata > ametadata > info.json > .ehdata > 其他 *.json。
// 应用方式：低优先级先应用、高优先级后应用覆盖。
// 目录形态只读本子根目录直接子项；归档形态可读任意深度。
// ─────────────────────────────────────────────────────────────

// mustWriteZipContent 写包含指定条目的 zip（条目名 → 内容）
func mustWriteZipContent(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建 zip 失败: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("写 zip 条目失败: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("写 zip 内容失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
}

func TestMetadataPriorityDir(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "metadata"), `{"title":"JSONTitle","gid":"100"}`)
	mustWriteFile(t, filepath.Join(dir, "ComicInfo.xml"),
		`<?xml version="1.0" encoding="utf-8"?><ComicInfo><Title>XMLTitle</Title></ComicInfo>`)

	meta := ParseDirMetadata(dir)
	if meta.Title != "XMLTitle" {
		t.Errorf("ComicInfo.xml 优先级应最高，Title=%q", meta.Title)
	}
	if meta.Source != "ComicInfo.xml" {
		t.Errorf("Source 应为 ComicInfo.xml，实际 %q", meta.Source)
	}
	if meta.GID != "100" {
		t.Errorf("低优先级 metadata 的 GID 仍应被保留（先到先得之外的空字段），实际 %q", meta.GID)
	}
}

func TestMetadataPriorityZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "x.zip")
	mustWriteZipContent(t, zipPath, map[string]string{
		"a/info.json":   `{"title":"InfoTitle","token":"t1"}`,
		"b/metadata":    `{"title":"MetaTitle"}`,
		"ComicInfo.xml": `<?xml version="1.0" encoding="utf-8"?><ComicInfo><Title>XMLTitle</Title></ComicInfo>`,
		"1.jpg":         "x",
	})

	meta := ParseZipMetadata(zipPath)
	if meta.Title != "XMLTitle" {
		t.Errorf("归档内 ComicInfo.xml 应最高优先，Title=%q", meta.Title)
	}
	if meta.Source != "ComicInfo.xml" {
		t.Errorf("Source 应为 ComicInfo.xml，实际 %q", meta.Source)
	}

	// 去掉 ComicInfo.xml → metadata 胜出
	dir2 := t.TempDir()
	zipPath2 := filepath.Join(dir2, "y.zip")
	mustWriteZipContent(t, zipPath2, map[string]string{
		"a/info.json": `{"title":"InfoTitle"}`,
		"b/metadata":  `{"title":"MetaTitle"}`,
	})
	meta2 := ParseZipMetadata(zipPath2)
	if meta2.Title != "MetaTitle" {
		t.Errorf("无 ComicInfo.xml 时 metadata 应胜出，Title=%q", meta2.Title)
	}
	if meta2.Source != "metadata" {
		t.Errorf("Source 应为 metadata，实际 %q", meta2.Source)
	}
}

func TestMetadataOtherJSONFallback(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "foo.json"), `{"title":"FooTitle"}`)

	meta := ParseDirMetadata(dir)
	if meta.Title != "FooTitle" {
		t.Errorf("其他 *.json 应作为兜底解析，Title=%q", meta.Title)
	}
	if meta.Source != "foo.json" {
		t.Errorf("Source 应为 foo.json，实际 %q", meta.Source)
	}
}

// TestMetadataDirIgnoresSubdirSidecar 目录形态只读本子根目录直接子项（D13 保持现状语义）。
func TestMetadataDirIgnoresSubdirSidecar(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "sub", "metadata"), `{"title":"SubTitle"}`)
	mustWriteFile(t, filepath.Join(dir, "1.jpg"), "x")

	meta := ParseDirMetadata(dir)
	if meta.Title != "" {
		t.Errorf("目录形态不应读取子目录侧车，Title=%q", meta.Title)
	}
	if meta.Source != "" {
		t.Errorf("无根侧车时 Source 应为空，实际 %q", meta.Source)
	}
}

// TestMetadataSidecarRank 优先级序数：固定名按列表顺序，其他 json 最低。
func TestMetadataSidecarRank(t *testing.T) {
	if sidecarRank("comicinfo.xml") >= sidecarRank("metadata") {
		t.Errorf("comicinfo.xml 应比 metadata 优先")
	}
	if sidecarRank(".ehdata") >= sidecarRank("other.json") {
		t.Errorf(".ehdata 应比其他 *.json 优先")
	}
	if !isMetadataSidecar("anything.json") || isMetadataSidecar("readme.txt") {
		t.Errorf("isMetadataSidecar 判定错误")
	}
}
