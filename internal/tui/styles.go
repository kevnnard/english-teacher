package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// colors holds one color per role from design/DESIGN.md. Dark values are the
// ones in DESIGN.md; light values are darker shades of the same hues so text
// stays readable on a light terminal. Primary text uses the terminal's own
// foreground, so it has no entry here.
type colors struct {
	accent, secondary, muted       color.Color
	success, warning, danger, info color.Color
	chrome                         color.Color // background of the rail, coach and status bar
}

func newColors(dark bool) colors {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	return colors{
		accent:    ld(c("#2E7DE9"), c("#7AA2F7")),
		secondary: ld(c("#6172B0"), c("#A0A0B0")),
		muted:     ld(c("#8990B3"), c("#6C6C7A")),
		success:   ld(c("#587539"), c("#9ECE6A")),
		warning:   ld(c("#8C6C3E"), c("#E0AF68")),
		danger:    ld(c("#C64343"), c("#F7768E")),
		info:      ld(c("#007197"), c("#7DCFFF")),
		chrome:    ld(c("#EEEEF2"), c("#1E1E26")),
	}
}

// roles are the text styles of DESIGN.md §3. The same roles exist twice: on
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
		inserted:  fg(p.success).Underline(true),
		deleted:   fg(p.danger).Strikethrough(true),
	}
}

// styles is everything View needs to paint the app.
type styles struct {
	main   roles // the content pane
	chrome roles // rail, coach and status bar
}

func newStyles(dark bool) styles {
	p := newColors(dark)
	return styles{main: newRoles(p, nil), chrome: newRoles(p, p.chrome)}
}
