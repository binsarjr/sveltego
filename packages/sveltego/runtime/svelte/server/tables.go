package server

// Boolean attribute table — mirrors svelte/src/utils.is_boolean_attribute.
// Source: WHATWG HTML spec boolean attributes.
var booleanAttrs = map[string]struct{}{
	"allowfullscreen":         {},
	"async":                   {},
	"autofocus":               {},
	"autoplay":                {},
	"checked":                 {},
	"controls":                {},
	"default":                 {},
	"disabled":                {},
	"formnovalidate":          {},
	"hidden":                  {},
	"indeterminate":           {},
	"inert":                   {},
	"ismap":                   {},
	"loop":                    {},
	"multiple":                {},
	"muted":                   {},
	"nomodule":                {},
	"novalidate":              {},
	"open":                    {},
	"playsinline":             {},
	"readonly":                {},
	"required":                {},
	"reversed":                {},
	"seamless":                {},
	"selected":                {},
	"webkitdirectory":         {},
	"defer":                   {},
	"disablepictureinpicture": {},
	"disableremoteplayback":   {},
}

// Void elements — mirrors svelte/src/utils.is_void.
var voidElements = map[string]struct{}{
	"area":   {},
	"base":   {},
	"br":     {},
	"col":    {},
	"embed":  {},
	"hr":     {},
	"img":    {},
	"input":  {},
	"link":   {},
	"meta":   {},
	"param":  {},
	"source": {},
	"track":  {},
	"wbr":    {},
}

// Raw text elements — mirrors svelte/src/utils.is_raw_text_element.
var rawTextElements = map[string]struct{}{
	"script": {},
	"style":  {},
}

func isBooleanAttribute(name string) bool {
	_, ok := booleanAttrs[name]
	return ok
}

// IsVoidElement reports whether tag is an HTML void element (no closing tag).
func IsVoidElement(tag string) bool {
	_, ok := voidElements[tag]
	return ok
}

// IsRawTextElement reports whether tag is a raw-text element (script/style).
func IsRawTextElement(tag string) bool {
	_, ok := rawTextElements[tag]
	return ok
}
