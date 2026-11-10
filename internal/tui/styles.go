package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// colors holds one color per role of the notebook palette
// (the notebook design): warm paper tones, an amber accent and a red margin
// line. Light values are darker shades of the same hues so text stays
// readable on a light terminal. Primary text uses the terminal's own
// foreground, so it has no entry here.
type colors struct {
	accent, secondary, muted       color.Color
	success, warning, danger, info color.Color
	margin                         color.Color // the notebook's margin line
	highlight                      color.Color // marker behind the word being learned
	chrome                         color.Color // background of the top and bottom bars
}

func newColors(dark bool) colors {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	return colors{
		accent:    ld(c("#9A6700"), c("#E6B450")),
		secondary: ld(c("#5C574C"), c("#C5BFAE")),
		muted:     ld(c("#8A8578"), c("#6F6A5E")),
		success:   ld(c("#5B7F23"), c("#A6CC70")),
		warning:   ld(c("#B05A00"), c("#FFB454")),
		danger:    ld(c("#C8383F"), c("#F07178")),
		info:      ld(c("#2F6DB5"), c("#73B8FF")),
		margin:    ld(c("#C0545A"), c("#B8545A")),
		highlight: ld(c("#F5E6BF"), c("#3A3424")),
		chrome:    ld(c("#EFEBE3"), c("#26282A")),
	}
}

// roles are the text styles of the notebook design. The same roles exist twice: on
// the plain canvas and on the tinted chrome, because a style must carry the
// background itself for it to survive the reset at the end of each segment.
type roles struct {
	base      lipgloss.Style // plain text; also pads lines with the background
	title     lipgloss.Style // bold + accent
	target    lipgloss.Style // the English being learned: bold, primary color
	value     lipgloss.Style
	label     lipgloss.Style // dim
	meta      lipgloss.Style // dim italic: pronunciation, dates, hints
	secondary lipgloss.Style
	accent    lipgloss.Style
	success   lipgloss.Style
	warning   lipgloss.Style
	danger    lipgloss.Style
	info      lipgloss.Style
	rule      lipgloss.Style // separators between regions
	word      lipgloss.Style // the English being learned, on the highlight marker
	prompt    lipgloss.Style // the Spanish prompt on the marker; never bold
	margin    lipgloss.Style // the notebook's margin line
	inserted  lipgloss.Style // letters the learner missed
	deleted   lipgloss.Style // letters the learner typed by mistake
}

func newRoles(p colors, bg color.Color) roles {
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}
	fg := func(c color.Color) lipgloss.Style { return base.Foreground(c) }
	return roles{
		base:      base,
		title:     fg(p.accent).Bold(true),
		target:    base.Bold(true),
		value:     base,
		label:     fg(p.muted),
		meta:      fg(p.muted).Italic(true),
		secondary: fg(p.secondary),
		accent:    fg(p.accent),
		success:   fg(p.success),
		warning:   fg(p.warning),
		danger:    fg(p.danger),
		info:      fg(p.info),
		rule:      fg(p.muted),
		word:      fg(p.accent).Background(p.highlight).Bold(true),
		prompt:    fg(p.accent).Background(p.highlight),
		margin:    fg(p.margin),
		inserted:  fg(p.success).Underline(true),
		deleted:   fg(p.danger).Strikethrough(true),
	}
}

// styles is everything View needs to paint the app.
type styles struct {
	main   roles // the content pane
	chrome roles // top and bottom bars
}

func newStyles(dark bool) styles {
	p := newColors(dark)
	return styles{main: newRoles(p, nil), chrome: newRoles(p, p.chrome)}
}
