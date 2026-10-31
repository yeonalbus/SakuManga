## 第二部分：过程记录

> 落档来源：本会话 JSONL（`.tmp-session.jsonl`，原为 zstd 压缩的 `session.v4.jsonl.zstd`），起点为第 1111 行 `user/message`（「首先是优化这个输入框」），至文件末行；共 17 条用户消息。
> 提交时间顺序：`9d1bc15` → `01ccd42` → `05d5a00` → `7ad65cf` → `9ac2bc2` → `aaf650e` → `c24a6a7`（`05d5a00` 为 embed 前端产物同步，随迭代 2 一并记录）。

### 迭代 1：备注输入框（3 行 textarea + 宽度/行数校验）

- **诉求**：备注编辑原先是 `modal.prompt` 的单行长条，输入多了看不见全部内容；显示上限从「约 60 个中文字符」收到 55 更保险；输入框要显示「已输入/上限」；超限不能静默丢弃，点确定要报错并拒绝保存；行数限 3 行、每行 19 个中文字符（珱垣实测值）。
- **方案**：新建专用弹窗 `BookmarkNoteModal.vue`（3 行 textarea，面板宽 420px）替代 `modal.prompt`；抽 `src/utils/noteLimit.ts` 统一宽度换算与校验（全角/中文/emoji 计 2 半角单位）；创建弹窗 `BookmarkCreateModal.vue` 备注同步改 textarea + 计数；卡片备注区 `max-width: 19em` 锁定每行 19 中文字符 + 3 行 line-clamp；后端 `services/scrape_bookmark.go` 加同规则校验（创建/更新两条路径），`ErrBookmarkNoteInvalid` → 400，防绕过前端。
- **决策**：中文字符与英文字符宽度确认为 1:2（1 中文 = 2 半角）；上限 110 半角单位（= 55 中文字符）；计数口径按珱垣后续指定改为「当前宽度/110（半角单位）」，不显示折算的中文字数；删除原 `maxlength="40"`——不再静默截断，超限由点确定时明确报错（计数标红 + 输入框描红 + 小字提示「输入超出限制（未保存）」，弹窗不关、不保存、不截断）。
- **实现**：`src/utils/noteLimit.ts`（新）、`src/components/BookmarkNoteModal.vue`（新）、`src/components/BookmarkCreateModal.vue`、`src/views/online/OnlineBookmarks.vue`、`backend/internal/services/scrape_bookmark.go`、`backend/internal/handlers/scrape_bookmark.go`、`backend/internal/services/scrape_bookmark_note_test.go`（新）；commit `9d1bc15`。
- **踩坑/bug**：单测用例最初写「55 个中文单行放行」，但单行 19 字限制会让它超限——改为三行 19+19+17；`backend/webui/dist` 是 embed 进 exe 的产物且已入库，前端构建后必须随提交同步，否则用户运行的程序仍是旧 UI。

### 迭代 2：侧栏统一（书签组改为书架同构）

- **诉求**：书签侧栏单独列出来很丑，要和首页、订阅等格式统一，不显示书签数、图标等无关内容；类似书架的布局，但书签不要折叠框；「检查更新」从书签标题行下移，放在书签与首页之间。
- **方案**：书签标题行复用书架同一个 `SidebarGroupHeader`（新增 `collapsible=false` 与 `activate` 变体），子项用书架子项样式（缩进、0.85rem）；去掉 🔖 图标、40 计数角标、🔄 探测按钮图标、置顶项右侧「追新/续扫」徽章，只留标题 + 有更新的红点；「检查更新」成为子项第一行。
- **决策**：先按「书架同构」落地，等珱垣看效果再定标识方案；role 徽章从侧栏移除（书签页卡片上仍保留、仍可点）。
- **实现**：`src/components/OnlineSidebar.vue`、`src/components/SidebarGroupHeader.vue`；commit `01ccd42`（代码）+ `05d5a00`（同步 `backend/webui/dist` 前端产物）。
- **踩坑/bug**：发现根 `dist/`（gitignore）与 `backend/webui/dist/`（入库、embed）是两份产物，`npm run build` 只更新前者；发布脚本 `build-release.bat` 第 2 步才复制过去。此次手动同步并单独提交，否则重新打包 exe 后仍是旧侧栏。

### 迭代 3：备注校验折算 + 书签并入在线模式组

- **诉求**：① 备注校验提示「每行最多 19 个中文字符」，但显示出来明明每行只有 19 个——误报，问要不要去掉每行硬限制；② 书签条目与在线模式其他条目隔得太开，希望「书签」作为该组第 7 项（首页/订阅/热门/排行榜/收藏/历史记录/书签）；③ 书签子项辨识度不高，问加徽章还是加 📌/🔄；④「检查更新」仿书架给颜色框或改底色文字色，并显示开启更新探测的数量。
- **决策**（珱垣拍板）：子项缩进 **32px**；「追新/续扫/兴趣」徽章**加回**（淡样式）；「检查更新」做成书架 `.all-shelf-btn` 同款边框按钮（透明底 + 1px 边框，hover 变蓝），文案 `检查更新（N）`，N = `updateWatch=true` 的书签数；不用 emoji 图标（后续要换矢量图标，避免二次返工）。
- **方案（bug 修复）**：根因是**软换行**——textarea 自动折行不产生 `\n`，旧校验把整段当「一行」算宽度 78 > 38 于是误报。改为「按每行 38 半角单位折算显示行数」：每段（按 `\n` 切分）`ceil(段宽/38)`，空段算 1 行，总折算行数 ≤ 3 且总宽 ≤ 110；错误文案改为「输入超出限制」/「最多 3 行（含自动折行）」，不再出现「每行最多 19 个中文字符」。后端 `validateScrapeBookmarkNote` 同步同一算法。
- **实现**：`src/utils/noteLimit.ts`、`src/components/OnlineSidebar.vue`（书签 router-link 并入在线模式组，取消独立 nav-group 的 20px 组间距）、`src/components/SidebarGroupHeader.vue`（`collapsible` 扩展回退，书架行为不变）、`backend/internal/services/scrape_bookmark.go`、`backend/internal/services/scrape_bookmark_note_test.go`；commit `7ad65cf`。
- **踩坑/bug**：折算规则的边界要成对验证——单段 78 宽通过、单段 110 宽（55 中文）通过、两段各 20 中文为 2+2=4 行拒绝、四段拒绝、三段 19+19+17 通过；总宽 110 上限必须保留，否则 57 中文（114 宽）也能凑成 3 行、越过 55 字上限。

### 迭代 4：名称/卡片/菜单设计（HTML 样板三版，无代码提交）

- **诉求**：① 标准 tag 搜索格式要显示翻译后的中文名作主标题，现在那串内容降为副标题小字、只给 1 行；② 名称新增自定义项（解决关键词搜索名称过长）；③ 去掉卡片底部配置栏，把置顶/更新探测/囤货/删除收进卡片右上角配置菜单，修改备注按钮放到备注框右上角（备注框加边界）；④ 先给 HTML 样板审核，减少返工。
- **方案**：静态样板 `DSPlans/plans/mockups/round49-bookmark-card.html`（暗色、纯静态、浏览器直接打开），v1（5 例）→ v2（6 例，按三类书签规则重排）→ v3（书签页整体布局 + 四区高度标注 + 菜单展开态 + 侧栏置顶列表）。
- **决策**（珱垣逐条拍板）：
  - 副标题**永远**是「位置字段 + 非默认筛选」（如 `搜索: artist:"fan no hitori$" · 中文`、`首页 · 仅删除`）；
  - 主标题优先级：自定义名称 > 标准 tag 翻译名 > 副标题内容本身；位置/关键词类主副标题相同也**两行都要显示**，不省略副标题；
  - 创建书签时不填名称（保持原创建流程），不满意再点菜单「修改名称」，改后持久继承；
  - 「修改名称」收进菜单，不在标题右侧单独放铅笔按钮；菜单保留「修改书签标记（追新/续扫/兴趣）」；
  - 徽章行（追新/已囤货/更新探测）保留为状态展示，位置在副标题与备注框之间；
  - 红点改为**金色框**；置顶书签在书签页真置顶（置顶区 + 1px 分隔线，置顶内仍按名称序）；不再显示 📌；有更新的书签在各自区域内置顶并加金框，侧栏置顶列表同样前置 + 金框；
  - 四区固定高度：标题 a=24 / 副标题 b=18 / 徽章 c=22 / 备注 d=76，合计 184 = 封面 160 + padding，防止内容变动推动卡片位移。
- **实现**：仅样板文件，无代码提交。
- **踩坑/bug**：样板阶段先确认了翻译数据可行性——后端 `TagEngine.TranslateTags` 只认无引号 key（`female:big breasts`），而书签 keyword 是 E 站 f_search 语法（`artist:"fan no hitori$"`，带引号与前缀通配符 `$`），必须先归一化（去引号、去尾部 `$`），否则查不到；最终采用前端 `useTagStore().translate` + `/tags/dictionary`（EhTagTranslation）。

### 迭代 5：名称翻译 + 卡片四区固定高度 + 置顶分区 + 金框

- **诉求**：按 v3 样板落实；翻译用「程序里已有」的代码；迁移方式由 AI 自行决定（珱垣表示「全都按搜索词迁移」也能接受）。
- **方案**：新增 `src/utils/bookmarkTitle.ts`（`extractBookmarkTagKey` 归一化 keyword、`bookmarkTitleWith` 主标题优先级）；拆出 `BookmarkCard.vue`（四区固定高度、金框、⚙ 菜单、备注框边框 + 框内右上角编辑）；`OnlineBookmarks.vue` 重写为「置顶区 → 分隔线 → 普通区」，区内「有更新在前 → 名称序（按主标题，含翻译名）」；`OnlineSidebar.vue` 置顶列表有更新前置 + 金框、只显示标题；store 删掉不再使用的 `sortedBookmarks`/`pinnedBookmarks`；`renameScrapeBookmark` 允许空名（留空即恢复默认显示）。
- **决策**：迁移时**无条件清空旧 name**（信息已提取进 role / 已囤货标记），name 自此仅表示「用户自定义名称」；用一次性内部标记 `NameReset` 保证只清一次，避免每次启动把用户后来的自定义名清掉；新书签创建时直接标记已迁移。
- **实现**：`src/utils/bookmarkTitle.ts`（新）、`src/components/BookmarkCard.vue`（新）、`src/views/online/OnlineBookmarks.vue`、`src/components/OnlineSidebar.vue`、`src/stores/scrapeBookmarksStore.ts`、`src/types/comic.ts`（name 注释）、`backend/internal/models/*`（NameReset 字段）、`backend/internal/services/scrape_bookmark*.go`（迁移清名、Create 标记、Rename 允许空名 + 单测断言）；commit `9ac2bc2`。
- **踩坑/bug**：`TagEngine`/词典可能尚未加载，书签页需 `tagStore.ensureDictionary()` 保证翻译可用；迁移清名若不用一次性标记会反复误清用户自定义名。

### 迭代 6：封面丢失修复 + 标记三项 chip + artist 豁免 + 左侧色条

- **诉求**：① 书签标记不要「点击切换」，直接给 追新|兴趣|续扫 三个切换项；② artist 类 tag 需要豁免（画师名有英文/日文/中文之别，如 `hirokoi00`→Hiro潶、`toraisi666`→日文假名、`jndfh`→只有英文）；③ 注意显示区域大小，「（点击切换）」已超出显示框；④ 更新过封面的书签、新建书签、迁移书签都没有封面——问是否 bug；⑤ 意见征询：要不要把 追新|兴趣|续扫 的颜色扩到卡片外框。
- **方案/定位**：封面不是数据丢失而是**渲染遗漏**——重写 `BookmarkCard.vue` 时封面区被写死成 `<div class="bm-cover">🖼</div>`，没有渲染 `<img>`（上一版 `OnlineBookmarks.vue` 里是有 `<img v-if="bm.anchor?.coverUrl">` 的）。顺带查出真实隐患：`useBookmarkMigrate.ts` 迁移写回时构造的新 anchor 没带 `coverUrl`（`TimeAnchorCandidate` 也没取），迁移过的书签会真的丢封面。
- **决策**：artist 豁免的口径为「有翻译用翻译，无翻译直接用 tag key 本身」，**不**再回退成 `搜索: artist:xxx$` 长串；不做「artist 一律用原名」的更彻底改法（珱垣确认现方案正确，原表述已退化）；不用「请求画廊详情页拿作者」方案（每本书签多一次请求，且详情页是 uploader 不是 tag 画师）；颜色方案采纳**左侧 3px 竖直色条**（追新=蓝、续扫=绿、兴趣=灰），不给整卡/外框上色——避免与金框「有更新」语义打架。
- **实现**：`src/utils/bookmarkTitle.ts`（artist 无翻译显示 key）、`src/utils/bookmarkTimeAnchor.ts`（`TimeAnchorCandidate` 加 `coverUrl`）、`src/composables/useBookmarkMigrate.ts`（迁移写回带上封面，取不到则保留旧封面）、`src/components/BookmarkCard.vue`（补 `<img>`、三项 chip、菜单项 ellipsis、左侧色条、删 `cycleRole` 改 `setRole`）；commit `aaf650e`。
- **踩坑/bug**：旧书签（加 `coverUrl` 之前创建）数据里本就没有封面 URL，仍显示占位，需重建书签或转移锚点；chip 点完不关菜单（还要能连续改别的），只有「删除」关菜单。

### 迭代 7：配置菜单被卡片 overflow 裁剪修复

- **诉求**：菜单里看不到三个 chip，问是不是改错了。
- **定位**：不是没渲染——构建产物里确认含 `bm-role-chip`/`roleChoices`；真因是 v4 给卡片加了 `overflow: hidden`（本意是让封面图圆角干净），而 ⚙ 菜单是绝对定位在卡片内，超出卡片底边的部分（分隔线、书签标记 chips、删除）被整块裁掉，菜单正好停在「标记已囤货」。
- **方案/决策**：去掉卡片自身的 `overflow: hidden`，封面圆角改由封面元素自己处理；菜单允许溢出卡片（绝对定位 + z-index，属下拉菜单正常行为）。
- **实现**：`src/components/BookmarkCard.vue`；commit `c24a6a7`。
- **踩坑/bug**：卡片内绝对定位菜单不能靠父级 `overflow` 裁剪做圆角——这条记入经验；另外「看不到某个菜单项」时先分辨是版本未更新（exe/dist 未重建）还是渲染/裁剪问题。

### 迭代 8：本次对话落档（无代码提交）

- **诉求**：把从「首先是优化这个输入框…」开始的整段对话落档到 `G:\SakuProjects\SakuManga\DSPlans\` 下（文件夹名自拟，作用是记录「没有正式落档、通过对话直接让 AI 做的改动」）；第一部分原封不动复制原话（禁止任何改动），第二部分记录方案/决策等过程；并指出 AI 不应凭记忆复述，应让子代理检索本地 DSH 会话文件。
- **决策**：文件夹命名取英文 `DshChats`（与 `plans`/`Problems`/`Research`/`VerNotes` 队列一致，不破坏命名风格）；文件 `round49-bookmark-ui-iteration.md`；原话部分用围栏代码块承载以保证逐字（`[截图]` 占位放在代码块外，不污染原文）；`agent/inbox/spliced` 里重复的同一用户消息一律忽略，只取 `user/message`。
- **实现**：会话原始文件为 `~/.dsh/sessions/--G-SakuProjects-SakuManga--/<session>/session.v4.jsonl.zstd`，先用 `zstd -d` 解压为 `.tmp-session.jsonl`，再用 Python 脚本（`DSPlans/DshChats/_extract.py`）从第 1111 行起提取 `user/message` 与 `assistant/message`，生成第一部分并拼接第二部分。
- **踩坑/bug**：全盘 `Select-String` 递归搜索太慢，应直接定位会话目录；`.jsonl.zstd` 需先解压才能解析。
