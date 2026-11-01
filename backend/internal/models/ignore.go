package models

import "time"

// IgnoredIdentifier 忽略标识（Round26 O2 建立；Round44 成员快照；Round51 改为按组成员）
//
// type=title：一个「疑似重复组」的成员集合（Round51 起按组成员，不再按名字）。
//   忽略时记录该簇的全部成员 id（SeenComicIDs）；匹配时按成员集合判定——
//   簇成员全在集合内 → 静默；出现集合外的新入库成员 → 重新列出并标注「新增 N 本」。
//   TitleKey/Artist 仅作展示（清单页标题），不参与匹配。
// type=gid：父画廊 gid（规则 3 例外，珱垣确认）。
//   忽略后，维护查重不再提示「旧版被新版取代（父画廊关系）可删除」。
//
// Round51（珱垣拍板）：忽略 = 「这一组（维护页上的那个簇）」，不是「名字」。
//   - 破坏性更新：升级后启动时清空旧语义条目（SnapshotVersion<2），用户重新忽略一次；
//   - type=comic（成员级忽略）移除：与「按组忽略」语义不兼容，且前端无入口。
//
// 作用域：仅疑似重复判定 + 规则 3 父子画廊；规则 1/2/4（同 GID/hash/内容签名）与更新检测不读取本表。
type IgnoredIdentifier struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"index" json:"type"`               // title | gid
	TitleKey  string    `gorm:"index" json:"titleKey,omitempty"` // type=title：展示用（归一化核心名）
	Artist    string    `json:"artist,omitempty"`                // type=title：展示用（画师）
	GID       string    `gorm:"index" json:"gid,omitempty"`      // type=gid：父画廊 gid
	Note      string    `json:"note,omitempty"`                  // 备注（可选）
	CreatedAt time.Time `json:"createdAt"`

	// Round51：type=title 的成员集合（JSON 数组，本地漫画 id；不外发，经 ListIgnoresWithMembers 转换后输出）
	SeenComicIDs string `gorm:"type:text" json:"-"`

	// Round51：语义版本。2 = 按组成员（新）；<2 = 旧逻辑（启动时清理）。
	SnapshotVersion int `gorm:"index" json:"-"`
}
