package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"orgmaid/internal/keys"
)

type helpEntry struct {
	key  string
	desc string
}

// helpContext tracks which page the help was opened from
type helpContext int

const (
	helpFromLog helpContext = iota
	helpFromDate
)

var helpSections = []struct {
	title   string
	entries []helpEntry
	context helpContext // which page this section applies to
}{
	{"日志页 | 内容列（默认停在这里）", []helpEntry{
		{"j / k", "下移、上移一项（上下方向键同效）"},
		{"J / K", "把当前项下移、上移一格，光标跟着走"},
		{"PgUp / PgDn", "翻页；Home / End 跳到首项、末项"},
		{"gg", "跳到第一项"},
		{"G", "跳到最后一项"},
		{"数字键", "跳到对应序号；300ms 内连按可拼成多位数"},
		{"i / a", "编辑当前项内容，光标落在开头 / 结尾（Enter 同 a）"},
		{"o", "下一行插入新项：开始为当前时间，收尾上一条未结束的，并进入编辑"},
		{"dd", "删除当前项"},
		{"y", "复制当前项到剪贴板（org-mode 格式，含 TODO、时间、标签）"},
		{"Y", "复制当日全部记录到剪贴板"},
		{"p", "把复制的项粘贴为最后一项并选中"},
		{"c", "打开日期选择页"},
		{"?", "打开本页"},
		{"q", "退出 orgmaid"},
	}, helpFromLog},
	{"日志页 | 一行的十一个停靠点", []helpEntry{
		{"Tab", "在停靠点之间移动，两端环绕（Shift+Tab 反向）"},
		{"停靠点", "序号/待办/计划/截止/开始/结束/耗时，共十一格，最后是内容"},
		{"0", "跳到序号停靠点（时间列上的 0 是个数字）"},
		{"j / k", "换项：移动光标，行的位置不变；时间列上也一样"},
		{"J / K", "挪本项：把当前项下移、上移一格，光标跟着走"},
		{"序号列 ↑ / ↓", "与内容列一样换项：序号只是位置，不参与挪动"},
		{"待办列 ↑ / ↓", "循环 TODO 状态：无 → TODO → DONE → 无"},
		{"计划/截止列 ↑ / ↓", "打开日期时间选择器，设置 SCHEDULED/DEADLINE"},
		{"计划/截止列 s", "设置为今天的日期"},
		{"计划/截止列 x", "清空 SCHEDULED 或 DEADLINE"},
		{"时间列 ↑ / ↓", "加减数值：时 ±1、分 ±5，到边界绕回（↑ 加、↓ 减）"},
		{"时间列数字键", "滚动填入：先十位后个位，两位满一个数值"},
		{"", "装不进时钟的那一位直接丢弃，不换手，也没有计时"},
		{"时间列 s", "把当前时间填进开始或结束时间"},
		{"时间列 x", "清空开始或结束时间（耗时列无效）"},
		{"耗时列 ↑ / ↓", "加减与填数字都写回结束时间，需要先有开始时间"},
		{"Enter / Esc", "从其他停靠点回到内容列（在内容列上 Enter 是编辑）"},
		{"行命令", "i a o dd y Y p c G J K ? q 在任何停靠点上都能按"},
	}, helpFromLog},
	{"日志页 | 编辑模式", []helpEntry{
		{"左右键", "移动光标"},
		{"Backspace", "删除光标前一个字符"},
		{"Enter / Esc", "提交并回到内容列"},
	}, helpFromLog},
	{"日志页 | TODO 与标签", []helpEntry{
		{"t", "循环当前项的 TODO 状态：无 → TODO → DONE → 无"},
		{",", "编辑当前项的标签，空格分隔，Enter 保存"},
		{"T", "打开全局 TODO 视图，查看所有未完成事项"},
	}, helpFromLog},
	{"日志页 | SCHEDULED 与 DEADLINE", []helpEntry{
		{"Tab 到计划列", "选中 SCHEDULED，显示 S（紧凑）或展开显示详情"},
		{"Tab 到截止列", "选中 DEADLINE，显示 D（紧凑）或展开显示详情"},
		{"↑ / ↓", "在计划/截止列按上下键打开日期时间选择器"},
		{"s", "在计划/截止列按 s 设置为今天"},
		{"x", "在计划/截止列按 x 清空该日期"},
		{"S（大写）", "在任意停靠点按 S 快速设置 SCHEDULED 为今天"},
		{"D（大写）", "在任意停靠点按 D 快速设置 DEADLINE 为今天"},
		{"显示格式", "紧凑：T/D/S；展开：TODO/DONE/S:日期时间/D:日期时间"},
	}, helpFromLog},
	{"全局 TODO 视图", []helpEntry{
		{"j / k", "上下移动"},
		{"t", "循环选中项的 TODO 状态"},
		{"Enter", "跳转到该日期并选中该项"},
		{"Esc / q", "返回日志页"},
	}, -1},
	{"日期选择页 | 列表模式", []helpEntry{
		{"j / k", "下移、上移一个日期（上下方向键同效）"},
		{"h / l", "上一页、下一页（左右方向键同效）"},
		{"/", "开始搜索，边输入边过滤"},
		{"Tab", "切换到日历视图"},
		{"Enter", "打开选中的日期"},
		{"c / Esc", "返回日志页"},
		{"?", "打开本页"},
		{"q", "退出 orgmaid"},
	}, helpFromDate},
	{"日期选择页 | 日历模式", []helpEntry{
		{"h / l / ← / →", "前一天 / 后一天"},
		{"j / k / ↓ / ↑", "上/下周同一天"},
		{"H / L", "上/下个月"},
		{"s", "跳到今天"},
		{"Tab", "切换到时间输入（小时），再按切换到分钟"},
		{"Enter", "确认选择（日历或时间）"},
		{"Esc", "返回日志页"},
		{"q", "退出 orgmaid"},
	}, helpFromDate},
	{"日期选择页 | 时间输入", []helpEntry{
		{"Tab", "在日历/小时/分钟之间切换"},
		{"↑ / ↓", "小时 ±1，分钟 ±5"},
		{"数字键", "滚动填入：先十位后个位"},
		{"s", "填入当前时间"},
		{"x", "清空当前字段"},
		{"Esc", "回到日历"},
	}, helpFromDate},
	{"搜索与新建日期", []helpEntry{
		{"2026-08", "按日期过滤，紧凑写法 202608 同样有效"},
		{"关键词", "按日志内容过滤，找出写过该词的日子"},
		{"20260801", "输入一个没有记录的完整日期，Enter 直接新建"},
		{"Esc", "清空搜索并取消"},
	}, helpFromDate},
	{"通用", []helpEntry{
		{"Ctrl+C", "在任意页面强制退出"},
		{"", "所有修改即时写入 ~/.orgmaid/orgmaid.org，退出无需保存"},
	}, -1}, // -1 means always show
}

const helpKeyWidth = 14

func (a *App) updateHelp(k tea.KeyMsg) tea.Cmd {
	a.status = ""
	step := 0

	if r, ok := keys.SingleRune(k); ok {
		switch r {
		case 'j':
			step = 1
		case 'k':
			step = -1
		case 'q', '?':
			if a.helpFrom == helpFromDate {
				a.page = pageDates
			} else {
				a.page = pageLog
			}
			return nil
		}
	}

	switch k.Type {
	case tea.KeyDown:
		step = 1
	case tea.KeyUp:
		step = -1
	case tea.KeyPgDown:
		step = a.listHeight()
	case tea.KeyPgUp:
		step = -a.listHeight()
	case tea.KeyHome:
		a.helpOffset = 0
		return nil
	case tea.KeyEnd:
		a.helpOffset = a.maxHelpOffset()
		return nil
	case tea.KeyEsc:
		if a.helpFrom == helpFromDate {
			a.page = pageDates
		} else {
			a.page = pageLog
		}
		return nil
	case tea.KeyCtrlC:
		return tea.Quit
	}

	if step != 0 {
		a.helpOffset = clamp(a.helpOffset+step, 0, a.maxHelpOffset())
	}
	return nil
}

func (a *App) maxHelpOffset() int {
	if n := len(a.helpLines(a.width)) - a.listHeight(); n > 0 {
		return n
	}
	return 0
}

func (a *App) viewHelp() string {
	lines := a.helpLines(a.width)
	height := a.listHeight()

	window := make([]string, 0, height)
	for i := a.helpOffset; i < len(lines) && len(window) < height; i++ {
		window = append(window, lines[i])
	}
	for len(window) < height {
		window = append(window, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		spread(a.width, titleStyle.Render("\uf128 快捷键"), dimStyle.Render("orgmaid")),
		divider(a.width),
		lipgloss.JoinVertical(lipgloss.Left, window...),
		divider(a.width),
		statusStyle.Width(a.width).Render(fit(
			"帮助 | j/k 或 上下键 滚动 | PgUp/PgDn 翻页 | q/Esc/? 返回日志页", a.width)),
	)
}

func (a *App) helpLines(width int) []string {
	sectionStyle := lipgloss.NewStyle().Bold(true).
		Foreground(fg(pal.Warn))
	descWidth := width - rowMargin - helpKeyWidth - 2

	var lines []string
	for _, section := range helpSections {
		// Filter sections based on context
		if section.context != -1 && section.context != a.helpFrom {
			continue
		}

		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, on(strings.Repeat(" ", rowMargin))+
			sectionStyle.Render(fit(section.title, width-rowMargin)))

		for _, entry := range section.entries {
			key := lipgloss.NewStyle().Width(helpKeyWidth).
				Foreground(fg(pal.Text)).
				Render(fit(entry.key, helpKeyWidth))
			desc := dimStyle.Render(fit(entry.desc, descWidth))
			lines = append(lines, on(strings.Repeat(" ", rowMargin))+key+on("  ")+desc)
		}
	}
	return lines
}
