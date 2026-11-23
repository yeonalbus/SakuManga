package services

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"SakuManga/internal/models"
)

func IsImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
		return true
	default:
		return false
	}
}

func IsArchive(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".zip", ".cbz", ".rar", ".7z":
		return true
	default:
		return false
	}
}

// IsSupportedArchive 可入库且可读的归档（D4：zip/cbz）
func IsSupportedArchive(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".zip" || ext == ".cbz"
}

// IsUnsupportedArchive 识别但不可读的归档（D4：rar/7z）
func IsUnsupportedArchive(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".rar" || ext == ".7z"
}

// GetCoverFromDir 取本子页表第一张未隐藏图的绝对路径（D11：封面 = 阅读器第 1 页）。
func GetCoverFromDir(dirPath string, hidden HiddenSet) (string, error) {
	pages, err := listImagesRecursive(dirPath)
	if err != nil {
		return "", err
	}
	for _, rel := range pages {
		if hidden.Paths[rel] {
			continue
		}
		return filepath.Join(dirPath, filepath.FromSlash(rel)), nil
	}
	return "", errors.New("未找到图片")
}

// GetCoverFromZip 读取 zip/cbz 页表第一张未隐藏图的图片数据（D11）。
func GetCoverFromZip(zipPath string, hidden HiddenSet) ([]byte, string, error) {
	pages, err := listImagesInZip(zipPath)
	if err != nil {
		return nil, "", err
	}
	var target string
	for _, name := range pages {
		if !hidden.Paths[name] {
			target = name
			break
		}
	}
	if target == "" {
		return nil, "", errors.New("压缩包内未找到图片")
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() || filepath.ToSlash(f.Name) != target {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, "", err
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return nil, "", err
		}
		return data, getContentType(target), nil
	}
	return nil, "", errors.New("压缩包内未找到图片")
}

// InvalidateCoverCache 删除某漫画的封面缓存文件（D11：隐藏页变化后必须立即失效；
// D14：删除记录时一并清理）。返回错误仅代表文件存在但删除失败。
func InvalidateCoverCache(comicID string) error {
	if comicID == "" {
		return nil
	}
	err := os.Remove(coverCachePath(comicID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Round14：离线封面缩略图缓存
//
// 根因：离线卡片封面每次请求都读取完整原图——ZIP/CBZ 每次 zip.OpenReader 打开
// 整个压缩包（如 192MB）解压第一张原图；散图文件夹直接 c.File 输出完整原图
// （可能数 MB）。离线首页 24 张/页并发请求 → 后端 IO/解码打满 → 界面卡顿。
// 方案：首次生成 480px 宽 JPEG 缩略图写入 <dataDir>/cover_cache/<id>.jpg，
// 后续直接输出缓存文件（带 Cache-Control/ETag，浏览器二次零请求）；
// 源文件 modtime 变化时自动重建。
// ─────────────────────────────────────────────────────────────

// CoverThumbWidth 封面缩略图目标宽度（Round14-D3=A：固定 480px）
const CoverThumbWidth = 480

// coverCacheDir 封面缓存目录（相对路径，main.go 已 chdir 到 exe 目录，与 manga.db 同级）
const coverCacheDir = "cover_cache"

// coverCacheMu 保护缓存目录创建
var coverCacheMu sync.Mutex

// coverCachePath 返回某 comic 的封面缓存路径
func coverCachePath(comicID string) string {
	return filepath.Join(coverCacheDir, comicID+".jpg")
}

// ensureCoverCacheDir 创建封面缓存目录（幂等）
func ensureCoverCacheDir() error {
	coverCacheMu.Lock()
	defer coverCacheMu.Unlock()
	return os.MkdirAll(coverCacheDir, 0o755)
}

// CoverCacheDir 返回封面缓存目录路径（供 handler 层复用，如在线封面代理缓存）
func CoverCacheDir() string {
	return coverCacheDir
}

// EnsureCoverCacheDir 创建封面缓存目录（导出版，供 handler 层调用）
func EnsureCoverCacheDir() error {
	return ensureCoverCacheDir()
}

// readCoverSource 读取封面源图（ZIP 或目录），返回字节 + 源修改时间（缓存失效依据）
func readCoverSource(comic models.OfflineComic, hidden HiddenSet) ([]byte, time.Time, error) {
	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	if fi.IsDir() {
		imgPath, err := GetCoverFromDir(comic.LocalPath, hidden)
		if err != nil {
			return nil, time.Time{}, err
		}
		data, err := os.ReadFile(imgPath)
		if err != nil {
			return nil, time.Time{}, err
		}
		imgFi, _ := os.Stat(imgPath)
		if imgFi != nil {
			return data, imgFi.ModTime(), nil
		}
		return data, fi.ModTime(), nil
	}
	if IsUnsupportedArchive(comic.LocalPath) {
		return nil, time.Time{}, ErrUnsupportedArchive
	}
	if IsSupportedArchive(comic.LocalPath) {
		data, _, err := GetCoverFromZip(comic.LocalPath, hidden)
		if err != nil {
			return nil, time.Time{}, err
		}
		return data, fi.ModTime(), nil
	}
	return nil, time.Time{}, errors.New("不支持的格式")
}

// decodeAndResize 解码图片并等比缩放到宽 CoverThumbWidth，输出 JPEG。
// 无法解码（如 AVIF）或无需缩放（原图更小）时返回 false，由调用方回退原图直传。
func decodeAndResize(data []byte) ([]byte, bool) {
	return decodeAndResizeTo(data, CoverThumbWidth)
}

// decodeAndResizeTo 通用版：解码图片并等比缩放到指定宽度，输出 JPEG（Round52：页缩略图复用）。
func decodeAndResizeTo(data []byte, width int) ([]byte, bool) {
	if width <= 0 {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	if w <= 0 || w <= width {
		return nil, false
	}
	h := bounds.Dy() * width / w
	if h <= 0 {
		return nil, false
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, h))
	// 盒式平均降采样（无需 x/image 依赖；列表缩略图画质足够）
	box := (w + width - 1) / width
	if box < 1 {
		box = 1
	}
	for y := 0; y < h; y++ {
		srcY := y * bounds.Dy() / h
		for x := 0; x < width; x++ {
			srcX := x * w / width
			var r, g, b, a, n uint32
			for dy := 0; dy < box; dy++ {
				for dx := 0; dx < box; dx++ {
					px := srcX + dx
					py := srcY + dy
					if px >= w {
						px = w - 1
					}
					if py >= bounds.Dy() {
						py = bounds.Dy() - 1
					}
					cr, cg, cb, ca := img.At(px, py).RGBA()
					r += cr >> 8
					g += cg >> 8
					b += cb >> 8
					a += ca >> 8
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			resized.SetRGBA(x, y, color.RGBA{
				R: uint8(r / n),
				G: uint8(g / n),
				B: uint8(b / n),
				A: uint8(a / n),
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 80}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// ─────────────────────────────────────────────────────────────
// Round24-P0-1/P0-2：封面生成性能优化
//
// P0-1 缓存命中短路：GetCoverThumb 先用 os.Stat 拿缓存文件与源 modtime（便宜），
//       命中直接返回 c.File，不再 readCoverSource（zip.OpenReader 整个包/解压首图）与解码缩放。
// P0-2 生成并发限流：全局信号量（2）限制解码缩放并发，防止列表页 24 张卡片首次请求
//       同时解码打满 N150 四核；per-comic 单飞锁防止同一封面并发重复生成。
// ─────────────────────────────────────────────────────────────

// coverGenSem 封面生成并发信号量（同时最多 2 个解码缩放任务）
var coverGenSem = make(chan struct{}, 2)

// coverGenMu 保护 coverGenerating 单飞表
var coverGenMu sync.Mutex
var coverGenerating = map[string]chan struct{}{} // comicID → 完成通知

// beginCoverGen 注册/等待单飞：返回 (ch, true) 表示已有其他请求正在生成，等待其完成；
// 返回 (ch, false) 表示由当前请求负责生成。
func beginCoverGen(comicID string) (chan struct{}, bool) {
	coverGenMu.Lock()
	defer coverGenMu.Unlock()
	if ch, ok := coverGenerating[comicID]; ok {
		return ch, true
	}
	ch := make(chan struct{})
	coverGenerating[comicID] = ch
	return ch, false
}

// endCoverGen 结束单飞并广播完成（幂等）
func endCoverGen(comicID string) {
	coverGenMu.Lock()
	if ch, ok := coverGenerating[comicID]; ok {
		delete(coverGenerating, comicID)
		close(ch)
	}
	coverGenMu.Unlock()
}

// coverSourceModTime 仅获取封面源图修改时间（不读取图片内容）：
// 文件夹 → 页表首张未隐藏图 stat；归档 → os.Stat 包文件。
func coverSourceModTime(comic models.OfflineComic, hidden HiddenSet) (time.Time, error) {
	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		return time.Time{}, err
	}
	if fi.IsDir() {
		imgPath, err := GetCoverFromDir(comic.LocalPath, hidden)
		if err != nil {
			return time.Time{}, err
		}
		imgFi, err := os.Stat(imgPath)
		if err != nil {
			return time.Time{}, err
		}
		return imgFi.ModTime(), nil
	}
	return fi.ModTime(), nil
}

// GetCoverThumb 获取离线漫画封面缩略图（Round14 + Round24 性能优化）：
//   - 缓存命中且源未变更 → 直接返回 (nil, cachePath, true)，零 zip IO / 零解码（P0-1）；
//   - 未命中 → 单飞去重（同封面并发只生成一次）+ 信号量限流（最多 2 个并发解码），生成写盘；
//   - 无需缩放/解码失败 → 返回 (srcData, "", false)，调用方原图直传。
func GetCoverThumb(comic models.OfflineComic) (data []byte, cachePath string, cached bool, err error) {
	cachePath = coverCachePath(comic.ID)
	pages, _ := GetPageList(comic.LocalPath)
	hidden := ParseHiddenSet(comic.HiddenPages, pages)
	srcMod, err := coverSourceModTime(comic, hidden)
	if err != nil {
		return nil, "", false, err
	}

	// P0-1：缓存命中短路（不读源图、不开 zip、不缩放）
	if fi, statErr := os.Stat(cachePath); statErr == nil {
		if !srcMod.IsZero() && !fi.ModTime().Before(srcMod) {
			TouchCoverCacheFile(cachePath) // Round39：打点最后使用时间（供 TTL 回收判活）
			return nil, cachePath, true, nil
		}
	}

	// P0-2a：per-comic 单飞——同一封面正在生成时，等待其完成再查缓存
	waitCh, waiting := beginCoverGen(comic.ID)
	if waiting {
		<-waitCh
		if _, statErr := os.Stat(cachePath); statErr == nil {
			TouchCoverCacheFile(cachePath) // Round39：同上，命中即打点
			return nil, cachePath, true, nil
		}
		// 对方生成失败/未写盘 → 继续自行生成
	}
	defer endCoverGen(comic.ID)

	// P0-2b：并发信号量限流（防 24 张卡片同时解码缩放打满 CPU）
	coverGenSem <- struct{}{}
	defer func() { <-coverGenSem }()

	srcData, _, err := readCoverSource(comic, hidden)
	if err != nil {
		return nil, "", false, err
	}

	thumb, ok := decodeAndResize(srcData)
	if !ok {
		return srcData, "", false, nil
	}
	if err := ensureCoverCacheDir(); err != nil {
		return nil, "", false, err
	}
	if err := os.WriteFile(cachePath, thumb, 0o644); err != nil {
		return nil, "", false, err
	}
	return nil, cachePath, true, nil
}
