// Package ast defines the typed syntax tree for Svelte 5 templates compiled
// by sveltego. Nodes carry source positions so codegen and the future LSP
// can attribute downstream errors back to the original .svelte source. The
// package is data-only: parsing lives in internal/parser, traversal lives
// in this package's visitor.go.
package ast

// Node is implemented by every concrete AST type. The unexported marker
// keeps the set closed so external packages cannot extend the tree.
type Node interface {
	Pos() Pos
	nodeMarker()
}

// AttrKind classifies an attribute's role on its element.
type AttrKind int

const (
	AttrStatic AttrKind = iota
	AttrDynamic
	AttrEventHandler
	AttrBind
	AttrUse
	AttrClassDirective
	AttrStyleDirective
	AttrLet
)

// String returns a stable name for the kind, used by golden fixtures and
// debug output.
func (k AttrKind) String() string {
	switch k {
	case AttrStatic:
		return "Static"
	case AttrDynamic:
		return "Dynamic"
	case AttrEventHandler:
		return "EventHandler"
	case AttrBind:
		return "Bind"
	case AttrUse:
		return "Use"
	case AttrClassDirective:
		return "ClassDirective"
	case AttrStyleDirective:
		return "StyleDirective"
	case AttrLet:
		return "Let"
	default:
		return "Unknown"
	}
}

// Fragment is a sequence of sibling nodes. The root of a parsed template
// is a Fragment.
type Fragment struct {
	P        Pos
	Children []Node
}

// Text is a literal run of characters between tags or mustaches.
type Text struct {
	P     Pos
	Value string
}

// Element is an HTML element or a Svelte component. Component is true when
// Name starts with an uppercase rune or contains a dot.
type Element struct {
	P           Pos
	Name        string
	Attributes  []Attribute
	Children    []Node
	SelfClosing bool
	Component   bool
}

// Attribute is one name/value pair (or directive) on an Element. Value is
// nil for boolean attributes.
type Attribute struct {
	P        Pos
	Name     string
	Value    AttributeValue
	Modifier string
	Kind     AttrKind
}

// Pos returns the attribute's source position.
func (a Attribute) Pos() Pos { return a.P }

// AttributeValue is the sealed sum of attribute value shapes:
// StaticValue, DynamicValue, InterpolatedValue.
type AttributeValue interface {
	Node
	attributeValueMarker()
}

// StaticValue is a literal attribute value, e.g. class="card".
type StaticValue struct {
	P     Pos
	Value string
}

// DynamicValue is a single Go expression bound to an attribute, e.g.
// class={Theme}.
type DynamicValue struct {
	P    Pos
	Expr string
}

// InterpolatedValue mixes literal Text and Mustache parts inside a quoted
// attribute value, e.g. class="card-{Theme}".
type InterpolatedValue struct {
	P     Pos
	Parts []Node
}

// Mustache is a {expr} interpolation in template body position. Expr holds
// the raw Go expression text; codegen validates it.
type Mustache struct {
	P    Pos
	Expr string
}

// IfBlock is {#if cond} ... {:else if ...} ... {:else} ... {/if}.
type IfBlock struct {
	P     Pos
	Cond  string
	Then  []Node
	Elifs []ElifBranch
	Else  []Node
}

// ElifBranch is one {:else if cond} arm of an IfBlock.
type ElifBranch struct {
	P    Pos
	Cond string
	Body []Node
}

// Pos returns the branch's source position.
func (e ElifBranch) Pos() Pos { return e.P }

// EachBlock is {#each iter as item, index (key)} ... {:else} ... {/each}.
type EachBlock struct {
	P     Pos
	Iter  string
	Item  string
	Index string
	Key   string
	Body  []Node
	Else  []Node
}

// AwaitBlock is {#await expr} ... {:then v} ... {:catch v} ... {/await}.
type AwaitBlock struct {
	P        Pos
	Expr     string
	Pending  []Node
	Then     []Node
	ThenVar  string
	Catch    []Node
	CatchVar string
}

// KeyBlock is {#key expr} ... {/key}, forcing re-mount on key change.
type KeyBlock struct {
	P    Pos
	Key  string
	Body []Node
}

// SnippetBlock is {#snippet name(params)} ... {/snippet}.
type SnippetBlock struct {
	P      Pos
	Name   string
	Params string
	Body   []Node
}

// RawHTML is the {@html expr} special form.
type RawHTML struct {
	P    Pos
	Expr string
}

// Const is the {@const stmt} special form.
type Const struct {
	P    Pos
	Stmt string
}

// Render is the {@render expr} special form.
type Render struct {
	P    Pos
	Expr string
}

// Script is the contents of a <script> block. Lang is the lang attribute
// (must be "go" for sveltego's regular scripts). Module marks Svelte 5
// `<script module>` blocks; their body is JS that runs once per module
// load and is passed through to the client compiler verbatim. Body is
// the raw script source.
type Script struct {
	P      Pos
	Lang   string
	Module bool
	Body   string
}

// Style is the raw contents of a <style> block. CSS parsing is deferred to
// downstream tooling.
type Style struct {
	P    Pos
	Body string
}

// Comment is an HTML comment captured verbatim.
type Comment struct {
	P     Pos
	Value string
}

func (n *Fragment) Pos() Pos          { return n.P }
func (n *Text) Pos() Pos              { return n.P }
func (n *Element) Pos() Pos           { return n.P }
func (n *StaticValue) Pos() Pos       { return n.P }
func (n *DynamicValue) Pos() Pos      { return n.P }
func (n *InterpolatedValue) Pos() Pos { return n.P }
func (n *Mustache) Pos() Pos          { return n.P }
func (n *IfBlock) Pos() Pos           { return n.P }
func (n *EachBlock) Pos() Pos         { return n.P }
func (n *AwaitBlock) Pos() Pos        { return n.P }
func (n *KeyBlock) Pos() Pos          { return n.P }
func (n *SnippetBlock) Pos() Pos      { return n.P }
func (n *RawHTML) Pos() Pos           { return n.P }
func (n *Const) Pos() Pos             { return n.P }
func (n *Render) Pos() Pos            { return n.P }
func (n *Script) Pos() Pos            { return n.P }
func (n *Style) Pos() Pos             { return n.P }
func (n *Comment) Pos() Pos           { return n.P }

// nodeMarker seals the Node interface to this package. It is intentionally
// unexported and never called directly; the type checker uses it to
// guarantee interface satisfaction is opt-in.
func (*Fragment) nodeMarker()          {}
func (*Text) nodeMarker()              {}
func (*Element) nodeMarker()           {}
func (*StaticValue) nodeMarker()       {}
func (*DynamicValue) nodeMarker()      {}
func (*InterpolatedValue) nodeMarker() {}
func (*Mustache) nodeMarker()          {}
func (*IfBlock) nodeMarker()           {}
func (*EachBlock) nodeMarker()         {}
func (*AwaitBlock) nodeMarker()        {}
func (*KeyBlock) nodeMarker()          {}
func (*SnippetBlock) nodeMarker()      {}
func (*RawHTML) nodeMarker()           {}
func (*Const) nodeMarker()             {}
func (*Render) nodeMarker()            {}
func (*Script) nodeMarker()            {}
func (*Style) nodeMarker()             {}
func (*Comment) nodeMarker()           {}

// attributeValueMarker seals the AttributeValue sum to the three concrete
// shapes declared in this file.
func (*StaticValue) attributeValueMarker()       {}
func (*DynamicValue) attributeValueMarker()      {}
func (*InterpolatedValue) attributeValueMarker() {}
