package services

import (
	"SakuHentai/internal/models"
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ─────────────────────────────────────────────────────────────
// 解压落地（计划第 4 步）
//
// 职责：把归档下载引擎拿到的 zip/cbz 解压到 extractDir；
//       - 解压过程中逐文件回写任务进度（DoneFiles / DoneBytes）
//       - 若解压目录缺少 metadata / ComicInfo.xml 则补写（含 gid/token，
//         保证离线书架能识别画廊身份、更新检测能工作）
//       - 按设置删除压缩包
// ─────────────────────────────────────────────────────────────

// extractArchive 解压 zip/cbz 到 extractDir，返回解压出的文件个数。
// ehService/account/ehSetting 为预留参数（供后续「解压后校验/补拉元数据」扩展）。
func extractArchive(task *models.DownloadTask, zipPath, extractDir string, deleteZip bool,
	db *gorm.DB, ehService *EHService, account *models.AccountSetting, ehSetting *models.EHSetting) (int, error) {

	if fi, err := os.Stat(zipPath); err != nil || fi.Size() == 0 {
		return 0, fmt.Errorf("压缩包不存在或为空: %s", zipPath)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("打开压缩包失败: %v", err)
	}
	defer zr.Close()

	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return 0, fmt.Errorf("创建解压目录失败: %v", err)
	}

	fileCount := 0
	var totalBytes int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// 安全解压：阻止 zip-slip 路径逃逸
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			log.Printf("%s [extract] 跳过危险路径（zip-slip）: %q", dlWarnTag, f.Name)
			continue
		}

		target := filepath.Join(extractDir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fileCount, fmt.Errorf("创建子目录失败 %q: %v", filepath.Dir(target), err)
		}

		rc, err := f.Open()
		if err != nil {
			return fileCount, fmt.Errorf("读取压缩包条目失败 %q: %v", f.Name, err)
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return fileCount, fmt.Errorf("写入解压文件失败 %q: %v", target, err)
		}
		n, cerr := io.Copy(out, rc)
		_ = out.Close()
		rc.Close()
		if cerr != nil {
			return fileCount, fmt.Errorf("解压 %q 失败: %v", f.Name, cerr)
		}

		fileCount++
		totalBytes += n

		// 进度回写（每 20 个文件或每 ~8MiB 落盘一次）
		if db != nil && task != nil && (fileCount%20 == 0 || totalBytes%(8*1024*1024) < 256*1024) {
			task.DoneFiles = fileCount
			task.DoneBytes = totalBytes
			task.UpdatedAt = time.Now()
			if err := db.Save(task).Error; err != nil {
				log.Printf("%s [extract] 任务 %s 解压进度保存失败: %v", dlErrTag, task.ID, err)
			}
		}
	}

	// 解压完成最终回写
	if db != nil && task != nil {
		task.DoneFiles = fileCount
		task.DoneBytes = totalBytes
		task.TotalFiles = fileCount
		task.UpdatedAt = time.Now()
		if err := db.Save(task).Error; err != nil {
			log.Printf("%s [extract] 任务 %s 解压完成进度保存失败: %v", dlErrTag, task.ID, err)
		}
	}

	// 补写 metadata / ametadata / ComicInfo.xml（含 gid/token，便于离线书架识别与更新检测）
	writeExtractMetadata(extractDir, task, ehService, account, ehSetting)

	// 关闭 zip 读取器，释放文件句柄（Windows 下句柄未释放会导致 os.Remove 被占用拒绝）
	_ = zr.Close()

	// 按设置删除压缩包
	if deleteZip {
		if err := os.Remove(zipPath); err != nil {
			log.Printf("%s [extract] 任务 %s 删除压缩包失败: %v", dlWarnTag, task.ID, err)
		} else {
			log.Printf("%s [extract] 任务 %s 已删除压缩包 %q（deleteZipAfterArchiveDownload=true）", dlLogTag, task.ID, zipPath)
		}
	}

	log.Printf("%s [extract] 任务 %s 解压完成：%d 个文件，%.2f MiB -> %q",
		dlLogTag, task.ID, fileCount, float64(totalBytes)/1024/1024, extractDir)
	return fileCount, nil
}

// writeExtractMetadata 若解压目录缺少 metadata/ametadata 或 ComicInfo.xml，则补写完整元数据。
// 优先通过 ehService 抓取画廊详情（含标签/作者/分类/评分等，参考 JHentai EHGalleryComicInfo），
// 抓取失败时回退到任务字段的最小集。已存在时保持原样（优先保留 E 站原生的 metadata）。
func writeExtractMetadata(extractDir string, task *models.DownloadTask,
	ehService *EHService, account *models.AccountSetting, ehSetting *models.EHSetting) {

	if task == nil {
		return
	}

	hasMeta := false
	hasXML := false
	if entries, err := os.ReadDir(extractDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if name == "metadata" || name == "ametadata" || strings.HasSuffix(name, ".json") {
				hasMeta = true
			}
			if name == "comicinfo.xml" {
				hasXML = true
			}
		}
	}
	if hasMeta && hasXML {
		return
	}

	// 抓取画廊详情构建完整元数据（失败仅告警，回退任务字段）
	var detail *GalleryDetailResult
	if ehService != nil && account != nil && task.GID != "" && task.Token != "" {
		d, err := ehService.FetchGalleryDetail(account, task.GID, task.Token, ehSetting)
		if err != nil || d == nil {
			log.Printf("%s [extract] 任务 %s 抓取画廊详情失败（回退任务字段元数据）: %v", dlWarnTag, task.ID, err)
		} else {
			detail = d
		}
	}

	galleryURL := ""
	if detail != nil {
		galleryURL = GetGalleryURL(account, ehSetting, detail.ID, detail.Token)
	} else if task.GID != "" && task.Token != "" {
		galleryURL = GetGalleryURL(account, ehSetting, task.GID, task.Token)
	}

	xmlMeta, jsonMeta := buildFullComicInfo(task, detail, galleryURL)

	if !hasMeta {
		data, err := json.MarshalIndent(jsonMeta, "", "  ")
		if err == nil {
			// JHentai 使用 ametadata 作为内部专有备份；我们同时写 metadata（兼容旧版/扫描器）
			for _, name := range []string{"metadata", "ametadata"} {
				if err := os.WriteFile(filepath.Join(extractDir, name), data, 0o644); err != nil {
					log.Printf("%s [extract] 补写 %s 失败: %v", dlWarnTag, name, err)
				} else {
					log.Printf("%s [extract] 已补写 %s（gid=%s title=%q tags=%d）", dlLogTag, name, task.GID, xmlMeta.Title, len(jsonMeta.Tags))
				}
			}
		}
	}

	if !hasXML {
		xmlData, err := xml.MarshalIndent(xmlMeta, "", "  ")
		if err == nil {
			xmlFull := append([]byte(xml.Header), xmlData...)
			if err := os.WriteFile(filepath.Join(extractDir, "ComicInfo.xml"), xmlFull, 0o644); err != nil {
				log.Printf("%s [extract] 补写 ComicInfo.xml 失败: %v", dlWarnTag, err)
			} else {
				log.Printf("%s [extract] 已补写 ComicInfo.xml（Title=%q Genre=%q Tags=%q）", dlLogTag, xmlMeta.Title, xmlMeta.Genre, xmlMeta.Tags)
			}
		}
	}
}
