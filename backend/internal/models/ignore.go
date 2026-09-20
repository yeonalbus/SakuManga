package models

import "time"

// IgnoredIdentifier 忽略标识（Round26 O2 忽略标记）
//
// type=title：疑似重复组的作品指纹（归一化核心名 + 画师，防撞名误伤）。
//   忽略后，增量查重中命中该指纹的疑似重复组不再出现；全量核对仍列出（带「已忽略」标记）。
// type=gid：父画廊 gid（规则 3 例外，珱垣确认）。
//   忽略后，维护查重不再提示「旧版被新版取代（父画廊关系）可删除」。
// type=comic：组内成员级忽略（Round26-2 珱垣需求）。
//   忽略后，该漫画不再参与名称级疑似重复聚类（被从疑似组中剔除）。
//
// Round44（成员快照 + 新增感知）：title 型条目额外记录**忽略当时的成员集合**（SeenComicIDs）。
//   - 查询时：簇成员全在快照内 → 静默跳过；出现快照外的本（忽略之后新入库）→ 照常列出并标注新增；
//   - 「确认新增」把快照刷新为当前匹配集合，随后继续静默；
//   - 语义由此从"这类作品永久静音"升级为"直到有变化为止"，从而不再需要"全量核对永久列出已忽略项"。
//
// 作用域：仅疑似重复判定 + 规则 3 父子画廊；规则 1/2/4（同 GID/hash/内容签名）与更新检测不读取本表。
type IgnoredIdentifier struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"index" json:"type"`                // title | gid | comic
	TitleKey  string    `gorm:"index" json:"titleKey,omitempty"`  // type=title：归一化核心名
	Artist    string    `json:"artist,omitempty"`                 // type=title：画师（来自 OnlineTags artist:xxx）
	GID       string    `gorm:"index" json:"gid,omitempty"`       // type=gid：父画廊 gid
	ComicID   string    `gorm:"index" json:"comicId,omitempty"`   // type=comic：被忽略的本地漫画 id
	Note      string    `json:"note,omitempty"`                   // 备注（可选）
	CreatedAt time.Time `json:"createdAt"`

	// Round44：title 型的成员快照（JSON 数组，本地漫画 id；不外发，经 ListIgnoresWithMembers 转换后输出）
	SeenComicIDs string `gorm:"type:text" json:"-"`
}
