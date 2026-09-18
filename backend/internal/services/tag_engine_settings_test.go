package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTagFixture 在 dir 下写入最小翻译库（db.raw.json），包含两条带中文名的标签
func writeTagFixture(t *testing.T, dir string) {
	t.Helper()
	fixture := `{
	  "data": [
	    {"namespace": "female", "data": {
	      "big breasts": {"name": "巨乳", "intro": ""},
	      "ahegao": {"name": "阿黑颜", "intro": ""}
	    }},
	    {"namespace": "parody", "data": {
	      "original": {"name": "原创", "intro": ""}
	    }}
	  ]
	}`
	if err := os.WriteFile(filepath.Join(dir, "db.raw.json"), []byte(fixture), 0644); err != nil {
		t.Fatalf("写测试用 db.raw.json 失败: %v", err)
	}
}

// dictNames 取出词典缓存中 key → name 的映射
func dictNames(t *testing.T, e *TagEngine) map[string]string {
	t.Helper()
	plain, _, _ := e.GetDictCache()
	if plain == nil {
		t.Fatal("词典缓存为空")
	}
	var brief []TagItemBrief
	if err := json.Unmarshal(plain, &brief); err != nil {
		t.Fatalf("词典缓存解析失败: %v", err)
	}
	out := make(map[string]string, len(brief))
	for _, b := range brief {
		out[b.Key] = b.Name
	}
	return out
}

// TestTagEngineApplySettingsRebuildsDictCache 中文翻译开关关闭后：
//  1. 词典缓存里的 name 退化为英文原文（前端 TagChip 因此显示原文）
//  2. 重新打开后中文恢复
//  3. 开关写入 config.json（持久化）
func TestTagEngineApplySettingsRebuildsDictCache(t *testing.T) {
	dir := t.TempDir()
	// config.json 路径是相对路径，切到临时目录避免污染仓库内配置
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	writeTagFixture(t, dir)

	e := &TagEngine{tags: map[string]*TagItem{}, dataDir: dir, EnableCN: true, EnableSort: true}
	e.LoadFromDisk()

	names := dictNames(t, e)
	if names["big breasts"] != "巨乳" {
		t.Fatalf("默认应为中文：got %q", names["big breasts"])
	}

	// 关闭中文翻译
	if err := e.ApplySettings(false, true); err != nil {
		t.Fatalf("ApplySettings 失败: %v", err)
	}
	names = dictNames(t, e)
	if names["big breasts"] != "big breasts" {
		t.Fatalf("关闭后词典 name 应退化为原文：got %q", names["big breasts"])
	}
	if names["original"] != "original" {
		t.Fatalf("关闭后词典 name 应退化为原文：got %q", names["original"])
	}

	// 持久化校验：config.json 中 enableCN=false
	data, err := os.ReadFile(filepath.Join(dir, configFilePath))
	if err != nil {
		t.Fatalf("config.json 未写入: %v", err)
	}
	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.json 解析失败: %v", err)
	}
	if cfg.TagEngine == nil || cfg.TagEngine.EnableCN == nil || *cfg.TagEngine.EnableCN {
		t.Fatalf("config.json 未持久化 enableCN=false: %s", string(data))
	}

	// 重新打开：中文恢复
	if err := e.ApplySettings(true, true); err != nil {
		t.Fatalf("ApplySettings 失败: %v", err)
	}
	if got := dictNames(t, e)["big breasts"]; got != "巨乳" {
		t.Fatalf("重新开启后应恢复中文：got %q", got)
	}
}

// TestConfigFileReadModifyWriteKeepsOtherFields 代理与标签引擎设置互不覆盖
// （修复前的 SetProxyURL 是整体覆盖写，会把 tagEngine 字段清掉）
func TestConfigFileReadModifyWriteKeepsOtherFields(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	if err := SetProxyURL("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("SetProxyURL 失败: %v", err)
	}
	if err := PersistTagEngineSettings(false, false); err != nil {
		t.Fatalf("PersistTagEngineSettings 失败: %v", err)
	}
	// 再次改代理：标签引擎字段必须仍在
	if err := SetProxyURL("socks5://127.0.0.1:1080"); err != nil {
		t.Fatalf("SetProxyURL 失败: %v", err)
	}

	cfg := loadConfigFile()
	if cfg.Proxy != "socks5://127.0.0.1:1080" {
		t.Fatalf("代理未保留: %q", cfg.Proxy)
	}
	if cfg.TagEngine == nil || cfg.TagEngine.EnableCN == nil || *cfg.TagEngine.EnableCN {
		t.Fatal("改代理后标签引擎开关被覆盖丢失")
	}
	if cfg.TagEngine.EnableSort == nil || *cfg.TagEngine.EnableSort {
		t.Fatal("改代理后 enableSort 字段被覆盖丢失")
	}
}

// TestInitTagEngineConfigAppliesPersisted 启动时套用 config.json 中的开关；
// 历史配置（无 tagEngine 字段）保持默认开启，不误判为「用户已关闭」
func TestInitTagEngineConfigAppliesPersisted(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	// 历史配置：只有 proxy
	if err := os.WriteFile(configFilePath, []byte(`{"proxy":""}`), 0644); err != nil {
		t.Fatal(err)
	}
	GlobalTagEngine.EnableCN = true
	GlobalTagEngine.EnableSort = true
	InitTagEngineConfig()
	if !GlobalTagEngine.EnableCN || !GlobalTagEngine.EnableSort {
		t.Fatal("历史配置应保持默认开启")
	}

	// 写入关闭状态后重启（再次 Init）应生效
	if err := PersistTagEngineSettings(false, true); err != nil {
		t.Fatal(err)
	}
	GlobalTagEngine.EnableCN = true // 模拟进程重启后的默认值
	InitTagEngineConfig()
	if GlobalTagEngine.EnableCN {
		t.Fatal("启动时应套用持久化的 enableCN=false")
	}
	if !GlobalTagEngine.EnableSort {
		t.Fatal("enableSort 未在持久化中保留为 true")
	}
	// 还原全局单例，避免影响其它测试
	GlobalTagEngine.EnableCN = true
	GlobalTagEngine.EnableSort = true
}
