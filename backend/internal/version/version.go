// Package version 提供应用版本与构建标识。
//
// 构建标识与 release 版本号（如 1.4.0）区分：release 版本用于发布语义，
// 构建标识用于精确定位「哪份源码、何时构建」，格式：YYYYMMDDHHMM-g<git短哈希>，
// 如 202608242215-g3f8a2c1。由 build-release.bat 通过 -ldflags -X 注入，
// 直接 go run / go build（未注入）时保持 "dev"。
package version

// Build 构建标识（时间戳 + git 短哈希），构建脚本注入；未注入为 "dev"
var Build = "dev"

// AppVersion 产品版本号（与 package.json 保持一致，发布时手动更新）
const AppVersion = "2.1.1"
