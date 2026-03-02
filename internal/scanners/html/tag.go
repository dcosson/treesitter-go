// Package html provides the external scanner for tree-sitter-html.
//
// Ported from tree-sitter-html/src/scanner.c and tag.h.
// The HTML external scanner handles 9 context-sensitive token types
// and maintains a stack of open tags to support implicit end tags.
package html

import (
	"unicode"
)

// TagType identifies an HTML tag. Known tags have specific enum values;
// unknown tags use Custom with a custom name string.
type TagType int

const (
	// Void elements (self-closing, no end tag allowed).
	Area TagType = iota
	Base
	Basefont
	Bgsound
	Br
	Col
	Command
	Embed
	Frame
	Hr
	Image
	Img
	Input
	Isindex
	Keygen
	Link
	Menuitem
	Meta
	Nextid
	Param
	Source
	Track
	Wbr
	endOfVoidTags // sentinel — all types < this are void

	// Non-void elements.
	A
	Abbr
	Address
	Article
	Aside
	Audio
	B
	Bdi
	Bdo
	Blockquote
	Body
	Button
	Canvas
	Caption
	Cite
	Code
	Colgroup
	Data
	Datalist
	Dd
	Del
	Details
	Dfn
	Dialog
	Div
	Dl
	Dt
	Em
	Fieldset
	Figcaption
	Figure
	Footer
	Form
	H1
	H2
	H3
	H4
	H5
	H6
	Head
	Header
	Hgroup
	HTML
	I
	Iframe
	Ins
	Kbd
	Label
	Legend
	Li
	Main
	Map_
	Mark
	Math
	Menu
	Meter
	Nav
	Noscript
	Object
	Ol
	Optgroup
	Option
	Output
	P
	Picture
	Pre
	Progress
	Q
	Rb
	Rp
	Rt
	Rtc
	Ruby
	S
	Samp
	Script
	Section
	Select
	Slot
	Small
	Span
	Strong
	Style
	Sub
	Summary
	Sup
	Svg
	Table
	Tbody
	Td
	Template
	Textarea
	Tfoot
	Th
	Thead
	Time
	Title
	Tr
	U
	Ul
	Var
	Video

	Custom // unknown/custom tag — uses customTagName

	end_ // sentinel
)

// Tag represents an open HTML element on the tag stack.
type Tag struct {
	Type          TagType
	CustomTagName string // non-empty only when Type == Custom
}

// tagMapEntry maps an uppercase tag name to a TagType.
type tagMapEntry struct {
	name    string
	tagType TagType
}

// tagMap is the lookup table for known HTML tags (uppercase names).
var tagMap = []tagMapEntry{
	{"AREA", Area}, {"BASE", Base}, {"BASEFONT", Basefont}, {"BGSOUND", Bgsound},
	{"BR", Br}, {"COL", Col}, {"COMMAND", Command}, {"EMBED", Embed},
	{"FRAME", Frame}, {"HR", Hr}, {"IMAGE", Image}, {"IMG", Img},
	{"INPUT", Input}, {"ISINDEX", Isindex}, {"KEYGEN", Keygen}, {"LINK", Link},
	{"MENUITEM", Menuitem}, {"META", Meta}, {"NEXTID", Nextid}, {"PARAM", Param},
	{"SOURCE", Source}, {"TRACK", Track}, {"WBR", Wbr},
	{"A", A}, {"ABBR", Abbr}, {"ADDRESS", Address}, {"ARTICLE", Article},
	{"ASIDE", Aside}, {"AUDIO", Audio}, {"B", B}, {"BDI", Bdi}, {"BDO", Bdo},
	{"BLOCKQUOTE", Blockquote}, {"BODY", Body}, {"BUTTON", Button},
	{"CANVAS", Canvas}, {"CAPTION", Caption}, {"CITE", Cite}, {"CODE", Code},
	{"COLGROUP", Colgroup}, {"DATA", Data}, {"DATALIST", Datalist}, {"DD", Dd},
	{"DEL", Del}, {"DETAILS", Details}, {"DFN", Dfn}, {"DIALOG", Dialog},
	{"DIV", Div}, {"DL", Dl}, {"DT", Dt}, {"EM", Em},
	{"FIELDSET", Fieldset}, {"FIGCAPTION", Figcaption}, {"FIGURE", Figure},
	{"FOOTER", Footer}, {"FORM", Form},
	{"H1", H1}, {"H2", H2}, {"H3", H3}, {"H4", H4}, {"H5", H5}, {"H6", H6},
	{"HEAD", Head}, {"HEADER", Header}, {"HGROUP", Hgroup}, {"HTML", HTML},
	{"I", I}, {"IFRAME", Iframe}, {"INS", Ins}, {"KBD", Kbd},
	{"LABEL", Label}, {"LEGEND", Legend}, {"LI", Li},
	{"MAIN", Main}, {"MAP", Map_}, {"MARK", Mark}, {"MATH", Math},
	{"MENU", Menu}, {"METER", Meter}, {"NAV", Nav}, {"NOSCRIPT", Noscript},
	{"OBJECT", Object}, {"OL", Ol}, {"OPTGROUP", Optgroup}, {"OPTION", Option},
	{"OUTPUT", Output}, {"P", P}, {"PICTURE", Picture}, {"PRE", Pre},
	{"PROGRESS", Progress}, {"Q", Q}, {"RB", Rb}, {"RP", Rp}, {"RT", Rt},
	{"RTC", Rtc}, {"RUBY", Ruby}, {"S", S}, {"SAMP", Samp},
	{"SCRIPT", Script}, {"SECTION", Section}, {"SELECT", Select}, {"SLOT", Slot},
	{"SMALL", Small}, {"SPAN", Span}, {"STRONG", Strong}, {"STYLE", Style},
	{"SUB", Sub}, {"SUMMARY", Summary}, {"SUP", Sup}, {"SVG", Svg},
	{"TABLE", Table}, {"TBODY", Tbody}, {"TD", Td}, {"TEMPLATE", Template},
	{"TEXTAREA", Textarea}, {"TFOOT", Tfoot}, {"TH", Th}, {"THEAD", Thead},
	{"TIME", Time}, {"TITLE", Title}, {"TR", Tr}, {"U", U}, {"UL", Ul},
	{"VAR", Var}, {"VIDEO", Video},
	{"CUSTOM", Custom},
}

// tagTypesNotAllowedInParagraphs lists tags that force-close a <p> element.
var tagTypesNotAllowedInParagraphs = []TagType{
	Address, Article, Aside, Blockquote, Details, Div, Dl,
	Fieldset, Figcaption, Figure, Footer, Form, H1, H2,
	H3, H4, H5, H6, Header, Hr, Main,
	Nav, Ol, P, Pre, Section,
}

// tagTypeForName returns the TagType for an uppercase tag name.
func tagTypeForName(name string) TagType {
	for _, entry := range tagMap {
		if entry.name == name {
			return entry.tagType
		}
	}
	return Custom
}

// tagForName returns a Tag for the given uppercase name string.
func tagForName(name string) Tag {
	tt := tagTypeForName(name)
	if tt == Custom {
		return Tag{Type: Custom, CustomTagName: name}
	}
	return Tag{Type: tt}
}

// isVoid returns true if the tag is a void element (no end tag).
func (t *Tag) isVoid() bool {
	return t.Type < endOfVoidTags
}

// eq returns true if two tags are equal.
func (t *Tag) eq(other *Tag) bool {
	if t.Type != other.Type {
		return false
	}
	if t.Type == Custom {
		return t.CustomTagName == other.CustomTagName
	}
	return true
}

// canContain returns whether this tag can contain the given child tag.
// This implements the HTML spec's implicit end tag rules.
func (t *Tag) canContain(child *Tag) bool {
	ct := child.Type
	switch t.Type {
	case Li:
		return ct != Li
	case Dt, Dd:
		return ct != Dt && ct != Dd
	case P:
		for _, notAllowed := range tagTypesNotAllowedInParagraphs {
			if ct == notAllowed {
				return false
			}
		}
		return true
	case Colgroup:
		return ct == Col
	case Rb, Rt, Rp:
		return ct != Rb && ct != Rt && ct != Rp
	case Optgroup:
		return ct != Optgroup
	case Tr:
		return ct != Tr
	case Td, Th:
		return ct != Td && ct != Th && ct != Tr
	default:
		return true
	}
}

// isAlnumOrDash returns true if ch is alphanumeric, '-', or ':'.
// Used for scanning tag names.
func isAlnumOrDash(ch int32) bool {
	return ch >= 0 && (unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '-' || ch == ':')
}

// toUpper uppercases a rune. HTML tag names are ASCII, so this
// effectively maps 'a'-'z' to 'A'-'Z' and leaves everything else unchanged.
func toUpper(ch int32) byte {
	if ch >= 'a' && ch <= 'z' {
		return byte(ch - 'a' + 'A')
	}
	return byte(ch)
}
