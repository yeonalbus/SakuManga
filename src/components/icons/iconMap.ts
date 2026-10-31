/**
 * emoji → Phosphor 图标映射表（B+C 方案）
 *
 * 原则：
 * - 普通位置用 weight="regular"（空心线性）
 * - 状态位激活时用 weight="fill"（实心）+ 状态色，见 §状态位清单
 * - 新增图标：在 ICONS 里补一行语义 key，模板里用 <AppIcon name="key" />
 *
 * 状态位（激活填充）清单：star(收藏/星级) heart(心) check(已下载/在清单)
 * bookmark(已书签) bell(更新探测) pin(已置顶) eyeClosed(已忽略) dot(在线)
 */
import type { Component } from 'vue'
import {
  PhArchive,
  PhArrowDown,
  PhArrowUp,
  PhArrowUpRight,
  PhArrowsClockwise,
  PhArrowsDownUp,
  PhBatteryFull,
  PhBell,
  PhBookOpen,
  PhBookmarkSimple,
  PhBooks,
  PhBroadcast,
  PhBroom,
  PhBug,
  PhCalendar,
  PhCards,
  PhCaretDoubleLeft,
  PhCaretDoubleRight,
  PhCaretDown,
  PhCaretLeft,
  PhCaretRight,
  PhCaretUp,
  PhChatText,
  PhChartBar,
  PhCheck,
  PhCheckCircle,
  PhCircle,
  PhClipboardText,
  PhClock,
  PhCloud,
  PhConfetti,
  PhCopy,
  PhCornersIn,
  PhCornersOut,
  PhCrown,
  PhDeviceMobile,
  PhDiceFive,
  PhDotsNine,
  PhDotsThree,
  PhDownloadSimple,
  PhEyeClosed,
  PhFile,
  PhFiles,
  PhFilmScript,
  PhFire,
  PhFloppyDisk,
  PhFolder,
  PhFolderOpen,
  PhFolders,
  PhGameController,
  PhGearSix,
  PhGlobe,
  PhHardDrives,
  PhHeart,
  PhHourglass,
  PhHouse,
  PhIdentificationCard,
  PhImage,
  PhImages,
  PhInfo,
  PhKey,
  PhLaptop,
  PhLayout,
  PhLightbulb,
  PhLightning,
  PhLink,
  PhList,
  PhListChecks,
  PhListDashes,
  PhLock,
  PhLockOpen,
  PhMagnifyingGlass,
  PhMagnifyingGlassPlus,
  PhMapPin,
  PhMedal,
  PhMonitor,
  PhMoon,
  PhNotebook,
  PhPalette,
  PhPaperPlane,
  PhPause,
  PhPencilSimple,
  PhPlay,
  PhPlus,
  PhProhibit,
  PhPushPin,
  PhQuestion,
  PhReceipt,
  PhScroll,
  PhSignOut,
  PhSlidersHorizontal,
  PhSmiley,
  PhSparkle,
  PhSpinnerGap,
  PhStar,
  PhSun,
  PhTag,
  PhTarget,
  PhTelevision,
  PhTrash,
  PhTray,
  PhTrophy,
  PhUser,
  PhUsers,
  PhWarning,
  PhWifiHigh,
  PhWrench,
  PhX,
  PhXCircle,
} from '@phosphor-icons/vue'

export const ICONS = {
  // ── 通用动作 ──
  close: PhX, // ✕ ✖ 关闭
  trash: PhTrash, // 🗑 删除
  check: PhCheck, // ✓ ✔ 确认
  refresh: PhArrowsClockwise, // 🔄 刷新
  download: PhDownloadSimple, // ⬇ 下载
  upload: PhArrowUp, // ⬆ 升级 / 回到顶部
  arrowDown: PhArrowDown, // ⬇ 排序方向
  plus: PhPlus, // ➕ 新建
  search: PhMagnifyingGlass, // 🔍 搜索
  searchPlus: PhMagnifyingGlassPlus, // 🔎 查重 / 高级搜索
  edit: PhPencilSimple, // ✎ ✏ 编辑
  save: PhFloppyDisk, // 💾 保存 / 本地文件
  copy: PhCopy, // 📋 复制
  link: PhLink, // 🔗 链接
  ban: PhProhibit, // 🚫 ⛔ 禁止 / 删除记录
  warning: PhWarning, // ⚠ 警告
  info: PhInfo, // ℹ 信息
  question: PhQuestion, // ❓ 未知
  loading: PhSpinnerGap, // ⏳ 加载中
  hourglass: PhHourglass, // ⏳ 等待
  play: PhPlay, // ▶ 播放
  pause: PhPause, // ⏸ 暂停
  target: PhTarget, // 🎯 目标
  lightning: PhLightning, // ⚡ 性能 / 强制
  lightbulb: PhLightbulb, // 💡 提示
  sparkle: PhSparkle, // ✨ 推荐
  confetti: PhConfetti, // 🎉 完成
  clipboard: PhClipboardText, // 📋 清单 / 复制

  // ── 导航 / 系统 ──
  home: PhHouse, // 🏠 首页
  online: PhGlobe, // 🌐 在线
  settings: PhGearSix, // ⚙ 设置
  menu: PhList, // 🍔 汉堡 / ☰ 菜单
  list: PhListChecks, // 📑 清单
  outline: PhListDashes, // ☰ 大纲
  filter: PhSlidersHorizontal, // 🎛 筛选
  user: PhUser, // 👤 用户
  users: PhUsers, // 👥 成员
  exit: PhSignOut, // 🚪 登出
  key: PhKey, // 🔑 登录
  lock: PhLock, // 🔒 锁定
  unlock: PhLockOpen, // 🔓 解锁
  crown: PhCrown, // 👑 冠军

  // ── 漫画 / 阅读 ──
  books: PhBooks, // 📚 书架 / 本地
  book: PhBookOpen, // 📖 阅读
  bookmark: PhBookmarkSimple, // 🔖 书签
  file: PhFile, // 📄 单页 / 文件
  files: PhFiles, // 🗂 文件堆
  images: PhImages, // 🖼 画廊
  image: PhImage, // 🖼 图片
  cards: PhCards, // 🎴 大卡片 / 🃏 抽卡
  film: PhFilmScript, // 🎞 Webtoon
  scroll: PhScroll, // 📜 历史 / 日志
  notebook: PhNotebook, // 📝 备注

  // ── 收藏 / 标记 ──
  star: PhStar, // ⭐ ★ ☆ 星级 / 偏好
  heart: PhHeart, // ❤ 收藏
  pin: PhPushPin, // 📌 置顶
  tag: PhTag, // 🏷 标签
  receipt: PhReceipt, // 🧾 已囤货
  eyeClosed: PhEyeClosed, // 🕶 忽略

  // ── 下载 / 存储 ──
  folder: PhFolder, // 📁 画廊 / 路径
  folderOpen: PhFolderOpen, // 📂 全部书架
  folders: PhFolders, // 🗂 维护对比
  archive: PhArchive, // 🗜 归档
  local: PhHardDrives, // 💾 本地 / 硬盘
  tray: PhTray, // 📥 加入书架

  // ── 在线 / 网络 ──
  wifi: PhWifiHigh, // 📶 网络
  broadcast: PhBroadcast, // 📡 监控
  cloud: PhCloud, // ☁ 词云

  // ── 工具 / 设置 ──
  palette: PhPalette, // 🎨 样式
  broom: PhBroom, // 🧹 清理
  wrench: PhWrench, // 🛠 维护
  ruler: PhLayout, // 📐 布局
  calendar: PhCalendar, // 📅 日期
  clock: PhClock, // 🕒 历史 / 时间
  bell: PhBell, // 🔔 通知
  fire: PhFire, // 🔥 热门
  medal: PhMedal, // 🥇🥈🥉 奖牌
  trophy: PhTrophy, // 🏆 排行榜
  gamepad: PhGameController, // 🎮 手柄
  sun: PhSun, // ☀️ 日间
  moon: PhMoon, // 🌙 夜间
  smiley: PhSmiley, // 😄 EH 网站

  // ── 状态 ──
  dot: PhCircle, // 🟢 状态点
  success: PhCheckCircle, // ✅ 成功
  error: PhXCircle, // ❌ 失败

  // ── 方向 / 折叠 ──
  arrowLeft: PhCaretLeft, // ‹ « 返回 / 上一页
  arrowRight: PhCaretRight, // › » 进入 / 下一页 / ❯ 折叠
  doubleLeft: PhCaretDoubleLeft, // « 首页
  doubleRight: PhCaretDoubleRight, // » 尾页
  caretUp: PhCaretUp, // ▲ 升序
  caretDown: PhCaretDown, // ▼ 降序
  sort: PhArrowsDownUp, // ↕ 排序
  expand: PhCornersOut, // 📺 全屏
  collapse: PhCornersIn, // 📉 退出全屏
  openExternal: PhArrowUpRight, // ↗ 新标签页打开提示
  more: PhDotsThree, // ⋯ 更多操作

  // ── 杂项 ──
  id: PhIdentificationCard, // 🪪 名片
  plane: PhPaperPlane, // ✈ 纯随机提示
  laptop: PhLaptop, // 💻 桌面设备
  monitor: PhMonitor, // 🖥 桌面布局
  television: PhTelevision, // 📺 全屏
  chat: PhChatText, // 💬 评论
  bug: PhBug, // 🐞 前端错误
  dice: PhDiceFive, // 🎲 随机抽卡
  mapPin: PhMapPin, // 📍 位置
  drag: PhDotsNine, // ⠿ 拖拽把手
  deviceMobile: PhDeviceMobile, // 📱 手机 / 平板
  chart: PhChartBar, // 📊 统计
  battery: PhBatteryFull, // 🔋 设备能力
} as const satisfies Record<string, Component>

/** 所有可用图标名（由 ICONS 的 key 推导，模板传错名会直接 type-check 报错） */
export type IconName = keyof typeof ICONS
