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

// GetCoverFromDir 递归查找文件夹（或深层子文件夹）中的第一张图片
func GetCoverFromDir(dirPath string) (string, error) {
	var firstImg string

	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && IsImage(d.Name()) {
			firstImg = path
			return filepath.SkipAll // 找到第一张图立即终止递归
		}
		return nil
	})

	if firstImg != "" {
		return firstImg, nil
	}
	if err != nil {
		return "", err
	}
	return "", errors.New("未找到图片")
}

// GetCoverFromZip 从 ZIP/CBZ 压缩包中流式读取第一张图片数据
func GetCoverFromZip(zipPath string) ([]byte, string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if !f.FileInfo().IsDir() && IsImage(f.Name) {
			rc, err := f.Open()
			if err != nil {
				return nil, "", err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, "", err
			}

			ext := strings.ToLower(filepath.Ext(f.Name))
			contentType := "image/jpeg"
			switch ext {
			case ".png":
				contentType = "image/png"
			case ".webp":
				contentType = "image/webp"
			case ".gif":
				contentType = "image/gif"
			}
			return data, contentType, nil
		}
	}
	return nil, "", errors.New("压缩包内未找到图片")
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
func readCoverSource(comic models.OfflineComic) ([]byte, time.Time, error) {
	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	if fi.IsDir() {
		imgPath, err := GetCoverFromDir(comic.LocalPath)
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
	if IsArchive(comic.LocalPath) {
		data, _, err := GetCoverFromZip(comic.LocalPath)
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
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	if w <= 0 || w <= CoverThumbWidth {
		return nil, false
	}
	h := bounds.Dy() * CoverThumbWidth / w
	if h <= 0 {
		return nil, false
	}
	resized := image.NewRGBA(image.Rect(0, 0, CoverThumbWidth, h))
	// 盒式平均降采样（无需 x/image 依赖；列表缩略图画质足够）
	box := (w + CoverThumbWidth - 1) / CoverThumbWidth
	if box < 1 {
		box = 1
	}
	for y := 0; y < h; y++ {
		srcY := y * bounds.Dy() / h
		for x := 0; x < CoverThumbWidth; x++ {
			srcX := x * w / CoverThumbWidth
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
// 文件夹 → WalkDir 找到首图即停再 stat（比读整图便宜得多）；归档 → os.Stat 包文件。
func coverSourceModTime(comic models.OfflineComic) (time.Time, error) {
	fi, err := os.Stat(comic.LocalPath)
	if err != nil {
		return time.Time{}, err
	}
	if fi.IsDir() {
		imgPath, err := GetCoverFromDir(comic.LocalPath)
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
	srcMod, err := coverSourceModTime(comic)
	if err != nil {
		return nil, "", false, err
	}

	// P0-1：缓存命中短路（不读源图、不开 zip、不缩放）
	if fi, statErr := os.Stat(cachePath); statErr == nil {
		if !srcMod.IsZero() && !fi.ModTime().Before(srcMod) {
			return nil, cachePath, true, nil
		}
	}

	// P0-2a：per-comic 单飞——同一封面正在生成时，等待其完成再查缓存
	waitCh, waiting := beginCoverGen(comic.ID)
	if waiting {
		<-waitCh
		if _, statErr := os.Stat(cachePath); statErr == nil {
			return nil, cachePath, true, nil
		}
		// 对方生成失败/未写盘 → 继续自行生成
	}
	defer endCoverGen(comic.ID)

	// P0-2b：并发信号量限流（防 24 张卡片同时解码缩放打满 CPU）
	coverGenSem <- struct{}{}
	defer func() { <-coverGenSem }()

	srcData, _, err := readCoverSource(comic)
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
