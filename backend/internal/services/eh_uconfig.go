package services

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"SakuHentai/internal/models"

	"github.com/PuerkitoBio/goquery"
)

// ============================================================
// EH 站点设置（uconfig.php）代理服务
// 复刻 E-Hentai 的配置界面：应用内直接读取 / 修改 / 保存，
// 不跳转到 uconfig.php 网页。含 profile 切换/新建/重命名/删除。
// ============================================================

// UConfigOption 单选/下拉的可选值
type UConfigOption struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Checked  bool   `json:"checked"`
	Disabled bool   `json:"disabled"`
}

// UConfigCategory 分类开关（ct_* hidden + 相邻 label）
type UConfigCategory struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
}

// UConfigCell 语言表格中的一格
type UConfigCell struct {
	Name    string `json:"name"`
	Checked bool   `json:"checked"`
}

// UConfigTableRow 语言表格的一行（一种语言）
type UConfigTableRow struct {
	Label string        `json:"label"`
	Cells []UConfigCell `json:"cells"`
}

// UConfigTable 语言表格（Original/Translated/Rewrite/All）
type UConfigTable struct {
	Columns []string          `json:"columns"`
	Rows    []UConfigTableRow `json:"rows"`
}

// UConfigField 一个结构化字段（radio/select/checkbox/text/textarea/category/language-table）
type UConfigField struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // radio | select | checkbox | text | textarea | category | language-table
	Label       string            `json:"label"`
	Hint        string            `json:"hint"`
	Description string            `json:"description"`
	Value       string            `json:"value"`
	Suffix      string            `json:"suffix"`
	Placeholder string            `json:"placeholder"`
	MaxLength   int               `json:"maxLength"`
	Checked     bool              `json:"checked"`
	Options     []UConfigOption   `json:"options"`
	Categories  []UConfigCategory `json:"categories"`
	Table       *UConfigTable     `json:"table"`
}

// UConfigSection 配置分组（对应页面中的 h2 标题）
type UConfigSection struct {
	Title  string         `json:"title"`
	Fields []UConfigField `json:"fields"`
}

// UConfigProfile profile 下拉选项
type UConfigProfile struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// UConfigData uconfig.php 的完整结构化描述
type UConfigData struct {
	Profiles        []UConfigProfile `json:"profiles"`
	SelectedProfile string           `json:"selectedProfile"`
	Sections        []UConfigSection `json:"sections"`
}

// hasAttr 判断元素是否带有某属性（用于 checked/selected/disabled）
func hasAttr(sel *goquery.Selection, name string) bool {
	_, exists := sel.Attr(name)
	return exists
}

// parseUConfigDoc 从已解析的 uconfig.php 文档提取结构化描述
func (s *EHService) parseUConfigDoc(doc *goquery.Document) *UConfigData {
	data := &UConfigData{}

	// 1. profile 下拉
	doc.Find("#profile_form select[name=profile_set] option").Each(func(i int, opt *goquery.Selection) {
		data.Profiles = append(data.Profiles, UConfigProfile{
			Value: opt.AttrOr("value", ""),
			Label: strings.TrimSpace(opt.Text()),
		})
		if hasAttr(opt, "selected") {
			data.SelectedProfile = opt.AttrOr("value", "")
		}
	})

	// 2. 主表单：包含 input[name=apply] 的 form
	var mainForm *goquery.Selection
	doc.Find("form").Each(func(i int, f *goquery.Selection) {
		if f.Find("input[name=apply]").Length() > 0 {
			mainForm = f
		}
	})
	if mainForm == nil {
		return data
	}

	// 3. 按文档顺序遍历 h2（分组标题）与 div.optouter（字段组）
	var sections []UConfigSection
	currentIdx := -1
	mainForm.Find("h2, div.optouter").Each(func(i int, sel *goquery.Selection) {
		if sel.Is("h2") {
			sections = append(sections, UConfigSection{Title: strings.TrimSpace(sel.Text())})
			currentIdx = len(sections) - 1
			return
		}
		fields := s.parseUConfigOptouter(sel)
		if len(fields) == 0 {
			return
		}
		if currentIdx < 0 {
			sections = append(sections, UConfigSection{})
			currentIdx = len(sections) - 1
		}
		sections[currentIdx].Fields = append(sections[currentIdx].Fields, fields...)
	})
	data.Sections = sections

	return data
}

// parseUConfigOptouter 解析一个 div.optouter，返回 0~n 个字段
func (s *EHService) parseUConfigOptouter(sel *goquery.Selection) []UConfigField {
	// 提取描述文本：所有 <p> 去掉控件后的文本
	desc := ""
	sel.Find("p").Each(func(i int, p *goquery.Selection) {
		clone := p.Clone()
		clone.Find("input, select, textarea, label").Remove()
		t := strings.TrimSpace(clone.Text())
		if t != "" {
			if desc != "" {
				desc += " "
			}
			desc += t
		}
	})
	desc = strings.TrimSpace(desc)

	// 分支 1：分类开关（hidden ct_*）
	if sel.Find("input[type=hidden][name^=ct_]").Length() > 0 {
		return []UConfigField{s.parseUConfigCategory(sel, desc)}
	}
	// 分支 2：语言排除表格（checkbox name^=xl_）
	if sel.Find("input[type=checkbox][name^=xl_]").Length() > 0 {
		return []UConfigField{s.parseUConfigLanguageTable(sel, desc)}
	}
	// 分支 3：vdtable 文本输入
	if sel.Find("table.vdtable").Length() > 0 {
		if f := s.parseUConfigVDTable(sel, desc); f != nil {
			return []UConfigField{*f}
		}
	}
	// 分支 4：通用（radio / select / checkbox / text / textarea）
	return s.parseUConfigGeneric(sel, desc)
}

// parseUConfigCategory 解析分类开关字段
func (s *EHService) parseUConfigCategory(sel *goquery.Selection, desc string) UConfigField {
	field := UConfigField{Type: "category", Description: desc}
	sel.Find("div.cs").Each(func(i int, div *goquery.Selection) {
		id := div.AttrOr("id", "")
		cat := strings.TrimSuffix(strings.TrimPrefix(id, "ct_"), "_div")
		val := sel.Find("input[name=ct_" + cat + "]").AttrOr("value", "0")
		field.Categories = append(field.Categories, UConfigCategory{
			Name:    cat,
			Label:   strings.TrimSpace(div.Text()),
			Checked: val == "1",
		})
	})
	return field
}

// parseUConfigLanguageTable 解析语言排除表格字段
func (s *EHService) parseUConfigLanguageTable(sel *goquery.Selection, desc string) UConfigField {
	field := UConfigField{Type: "language-table", Description: desc}
	table := sel.Find("table").First()
	if table.Length() == 0 {
		return field
	}

	var columns []string
	table.Find("th").Each(func(i int, th *goquery.Selection) {
		if t := strings.TrimSpace(th.Text()); t != "" {
			columns = append(columns, t)
		}
	})

	var rows []UConfigTableRow
	table.Find("tr").Each(func(i int, tr *goquery.Selection) {
		if tr.Find("th").Length() > 0 {
			return // 跳过表头行
		}
		cells := tr.Find("td")
		if cells.Length() < 2 {
			return
		}
		row := UConfigTableRow{Label: strings.TrimSpace(cells.Eq(0).Text())}
		for j := 1; j < cells.Length(); j++ {
			td := cells.Eq(j)
			input := td.Find("input[type=checkbox]")
			if input.Length() == 0 {
				row.Cells = append(row.Cells, UConfigCell{Name: "", Checked: false})
				continue
			}
			row.Cells = append(row.Cells, UConfigCell{
				Name:    input.AttrOr("name", ""),
				Checked: hasAttr(input, "checked"),
			})
		}
		rows = append(rows, row)
	})

	field.Table = &UConfigTable{Columns: columns, Rows: rows}
	return field
}

// parseUConfigVDTable 解析 vdtable 文本输入字段（tp/ru/wt/ft/vp）
func (s *EHService) parseUConfigVDTable(sel *goquery.Selection, desc string) *UConfigField {
	tr := sel.Find("table.vdtable tr").First()
	if tr.Length() == 0 {
		return nil
	}
	tds := tr.Find("td")
	if tds.Length() < 2 {
		return nil
	}
	input := tds.Eq(0).Find("input")
	if input.Length() == 0 {
		return nil
	}

	field := UConfigField{
		Type:        "text",
		Description: desc,
		Name:        input.AttrOr("name", ""),
		Value:       input.AttrOr("value", ""),
		Placeholder: input.AttrOr("placeholder", ""),
		Hint:        strings.TrimSpace(tds.Eq(1).Text()),
	}
	// 后缀：第一个 td 去掉 input 后的文本（如 "%"、"px"）
	clone := tds.Eq(0).Clone()
	clone.Find("input").Remove()
	field.Suffix = strings.TrimSpace(clone.Text())
	if ml, err := strconv.Atoi(input.AttrOr("maxlength", "")); err == nil {
		field.MaxLength = ml
	}
	return &field
}

// parseUConfigGeneric 解析通用字段（radio/select/checkbox/text/textarea）
func (s *EHService) parseUConfigGeneric(sel *goquery.Selection, desc string) []UConfigField {
	var fields []UConfigField

	// textarea
	sel.Find("textarea").Each(func(i int, ta *goquery.Selection) {
		fields = append(fields, UConfigField{
			Type:        "textarea",
			Description: desc,
			Name:        ta.AttrOr("name", ""),
			Value:       ta.Text(),
		})
	})

	// radio：按 name 分组
	seenRadio := map[string]bool{}
	sel.Find("input[type=radio]").Each(func(i int, input *goquery.Selection) {
		name := input.AttrOr("name", "")
		if name == "" || seenRadio[name] {
			return
		}
		seenRadio[name] = true
		fields = append(fields, s.parseRadioGroup(sel, name, desc))
	})

	// select
	sel.Find("select").Each(func(i int, sl *goquery.Selection) {
		name := sl.AttrOr("name", "")
		if name == "" {
			return
		}
		field := UConfigField{Type: "select", Description: desc, Name: name}
		parent := sl.Parent()
		if parent.Length() > 0 {
			clone := parent.Clone()
			clone.Find("select").Remove()
			field.Label = strings.TrimSpace(clone.Text())
		}
		sl.Find("option").Each(func(j int, opt *goquery.Selection) {
			field.Options = append(field.Options, UConfigOption{
				Value:   opt.AttrOr("value", ""),
				Label:   strings.TrimSpace(opt.Text()),
				Checked: hasAttr(opt, "selected"),
			})
		})
		fields = append(fields, field)
	})

	// checkbox（排除语言表格 xl_*）
	seenCheckbox := map[string]bool{}
	sel.Find("input[type=checkbox]").Each(func(i int, input *goquery.Selection) {
		name := input.AttrOr("name", "")
		if name == "" || strings.HasPrefix(name, "xl_") || seenCheckbox[name] {
			return
		}
		seenCheckbox[name] = true
		label := ""
		if parent := input.Parent(); parent.Is("label") {
			clone := parent.Clone()
			clone.Find("input, span").Remove()
			label = strings.TrimSpace(clone.Text())
		}
		fields = append(fields, UConfigField{
			Type:        "checkbox",
			Description: desc,
			Name:        name,
			Label:       label,
			Checked:     hasAttr(input, "checked"),
		})
	})

	// text input（rx/ry、favorite_* 等）
	seenText := map[string]bool{}
	sel.Find("input[type=text]").Each(func(i int, input *goquery.Selection) {
		name := input.AttrOr("name", "")
		if name == "" || seenText[name] {
			return
		}
		seenText[name] = true
		field := UConfigField{
			Type:        "text",
			Description: desc,
			Name:        name,
			Value:       input.AttrOr("value", ""),
			Placeholder: input.AttrOr("placeholder", ""),
		}
		if ml, err := strconv.Atoi(input.AttrOr("maxlength", "")); err == nil {
			field.MaxLength = ml
		}
		parent := input.Parent()
		if parent.Is("td") {
			// 表格文本：label 来自前一列，suffix 来自本列去掉 input
			if prev := parent.Prev(); prev.Length() > 0 {
				field.Label = strings.TrimSpace(prev.Text())
			}
			clone := parent.Clone()
			clone.Find("input").Remove()
			field.Suffix = strings.TrimSpace(clone.Text())
		} else if parent.Parent().Is("div") {
			// 收藏分类：label 来自 div.i 的 title
			outer := parent.Parent()
			if title := outer.Find("div.i").AttrOr("title", ""); title != "" {
				field.Label = title
			}
		}
		fields = append(fields, field)
	})

	return fields
}

// parseRadioGroup 解析同一 name 的 radio 组
func (s *EHService) parseRadioGroup(sel *goquery.Selection, name, desc string) UConfigField {
	field := UConfigField{Type: "radio", Description: desc, Name: name}

	sel.Find(`input[type=radio][name="` + name + `"]`).Each(func(i int, input *goquery.Selection) {
		label := ""
		if parent := input.Parent(); parent.Is("label") {
			clone := parent.Clone()
			clone.Find("input, span").Remove()
			label = strings.TrimSpace(clone.Text())
		}
		field.Options = append(field.Options, UConfigOption{
			Value:    input.AttrOr("value", ""),
			Label:     label,
			Checked:   hasAttr(input, "checked"),
			Disabled:  hasAttr(input, "disabled"),
		})
	})

	// 分组标签：若 radio 位于 table 中（如 ts/tr），取所在行第一列文本（Size: / Rows:）
	if field.Label == "" {
		if tr := sel.Find(`tr:has(input[type=radio][name="` + name + `"])`).First(); tr.Length() > 0 {
			if td := tr.Find("td").First(); td.Length() > 0 {
				field.Label = strings.TrimSpace(td.Text())
			}
		}
	}
	return field
}

// FetchUConfig 读取 uconfig.php 并返回结构化配置描述
func (s *EHService) FetchUConfig(account *models.AccountSetting, setting *models.EHSetting) (*UConfigData, error) {
	client, err := s.BuildClient(account)
	if err != nil {
		return nil, fmt.Errorf("构造客户端失败: %v", err)
	}
	base := GetBaseURL(account, setting)
	html, err := s.fetchHTML(client, base+"uconfig.php")
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("解析 uconfig 页面失败: %v", err)
	}
	return s.parseUConfigDoc(doc), nil
}

// SaveUConfig 保存 uconfig 配置或执行 profile 操作，成功后返回最新解析结果
//   - profile: 目标 profile 的 value（必填）
//   - action: "" 表示保存配置；可选 "rename" | "create" | "default" | "delete"
//   - profileName: action 为 rename/create 时需要的名字
//   - fields: 配置字段 name→value（checkbox 勾选才含，值为 on；category 恒含 0/1；apply 由本函数附加）
func (s *EHService) SaveUConfig(account *models.AccountSetting, setting *models.EHSetting,
	profile, action, profileName string, fields map[string]string) (*UConfigData, error) {

	client, err := s.BuildClient(account)
	if err != nil {
		return nil, fmt.Errorf("构造客户端失败: %v", err)
	}
	base := GetBaseURL(account, setting)
	uconfigURL := base + "uconfig.php"

	form := url.Values{}
	form.Set("profile_set", profile)
	if action != "" {
		form.Set("profile_action", action)
		if profileName != "" {
			form.Set("profile_name", profileName)
		}
	} else if len(fields) > 0 {
		// 保存配置：提交全部字段并附带 apply
		for k, v := range fields {
			form.Set(k, v)
		}
		form.Set("apply", "Apply")
	}
	// else：仅切换 profile（只提交 profile_set）

	req, err := http.NewRequest("POST", uconfigURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %v", err)
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", uconfigURL)
	req.Header.Set("Origin", strings.TrimSuffix(base, "/"))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("提交 uconfig 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 uconfig 响应失败: %v", err)
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("解析 uconfig 响应失败: %v", err)
	}

	// profile 操作错误信息显示在 #profile_action 中
	if msg := strings.TrimSpace(doc.Find("#profile_action").Text()); msg != "" {
		return nil, fmt.Errorf("E 站处理失败: %s", msg)
	}

	return s.parseUConfigDoc(doc), nil
}
