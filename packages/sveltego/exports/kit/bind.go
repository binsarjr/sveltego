package kit

import (
	"errors"
	"mime/multipart"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultMaxFormMemory is the maxMemory value [RequestEvent.BindMultipart]
// uses when the caller passes a non-positive limit. Multipart parts
// larger than this go to disk via the stdlib temp-file path.
const DefaultMaxFormMemory int64 = 32 << 20 // 32 MiB

// BindError aggregates per-field coercion failures from [RequestEvent.BindForm]
// and [RequestEvent.BindMultipart]. FieldErrors is keyed by the form
// name (the `form:"..."` tag value, falling back to the lowercased
// struct field name).
//
// FieldErrors is a public map; callers must not mutate it after BindForm
// or BindMultipart returns -- the behavior of doing so is undefined.
type BindError struct {
	FieldErrors map[string]string
}

// Error implements error. The string is stable across runs because
// fields are sorted before joining.
func (b *BindError) Error() string {
	if b == nil || len(b.FieldErrors) == 0 {
		return "bind: no field errors"
	}
	keys := make([]string, 0, len(b.FieldErrors))
	for k := range b.FieldErrors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+b.FieldErrors[k])
	}
	return "bind: " + strings.Join(parts, "; ")
}

// BindForm parses the request's url-encoded body into dst by reflecting
// over its `form` tags. Numeric / bool / time.Time / []string fields
// are coerced; coercion errors aggregate into a [*BindError]. dst must
// be a non-nil pointer to a struct.
func (e *RequestEvent) BindForm(dst any) error {
	if e == nil || e.Request == nil {
		return errors.New("bind: nil request")
	}
	if err := e.Request.ParseForm(); err != nil {
		return err
	}
	return bindStruct(dst, e.Request.PostForm, nil)
}

// BindMultipart parses the request's multipart body into dst. File
// fields receive `*multipart.FileHeader` or `[]*multipart.FileHeader`.
// maxMemory follows net/http semantics (parts above the limit spill to
// temp files); a non-positive value uses [DefaultMaxFormMemory].
func (e *RequestEvent) BindMultipart(dst any, maxMemory int64) error {
	if e == nil || e.Request == nil {
		return errors.New("bind: nil request")
	}
	if maxMemory <= 0 {
		maxMemory = DefaultMaxFormMemory
	}
	if err := e.Request.ParseMultipartForm(maxMemory); err != nil {
		return err
	}
	form := e.Request.MultipartForm
	var values map[string][]string
	var files map[string][]*multipart.FileHeader
	if form != nil {
		values = form.Value
		files = form.File
	}
	return bindStruct(dst, values, files)
}

// Files returns the multipart file map for the request, or nil when no
// multipart form has been parsed yet. Call after [RequestEvent.BindMultipart]
// or [http.Request.ParseMultipartForm].
func (e *RequestEvent) Files() map[string][]*multipart.FileHeader {
	if e == nil || e.Request == nil || e.Request.MultipartForm == nil {
		return nil
	}
	return e.Request.MultipartForm.File
}

type fieldInfo struct {
	index    int
	formName string
	typ      reflect.Type
}

var (
	bindCache    sync.Map // reflect.Type -> []fieldInfo
	timeType     = reflect.TypeOf(time.Time{})
	fileHdrType  = reflect.TypeOf((*multipart.FileHeader)(nil))
	fileHdrSlice = reflect.TypeOf([]*multipart.FileHeader(nil))
)

func bindStruct(dst any, values map[string][]string, files map[string][]*multipart.FileHeader) error {
	rv := reflect.ValueOf(dst)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("bind: dst must be non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return errors.New("bind: dst must point to struct")
	}
	infos := fieldsFor(rv.Type())
	errs := map[string]string{}
	for _, fi := range infos {
		fv := rv.Field(fi.index)
		if isFileField(fi.typ) {
			assignFiles(fv, files[fi.formName])
			continue
		}
		raw := values[fi.formName]
		if len(raw) == 0 {
			continue
		}
		if err := assignField(fv, raw); err != nil {
			errs[fi.formName] = err.Error()
		}
	}
	if len(errs) > 0 {
		return &BindError{FieldErrors: errs}
	}
	return nil
}

func fieldsFor(t reflect.Type) []fieldInfo {
	if v, ok := bindCache.Load(t); ok {
		return v.([]fieldInfo)
	}
	var out []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("form")
		if tag == "-" {
			continue
		}
		name := tag
		if name == "" {
			name = strings.ToLower(sf.Name)
		}
		out = append(out, fieldInfo{
			index:    i,
			formName: name,
			typ:      sf.Type,
		})
	}
	bindCache.Store(t, out)
	return out
}

func isFileField(t reflect.Type) bool {
	return t == fileHdrType || t == fileHdrSlice
}

func assignFiles(fv reflect.Value, hdrs []*multipart.FileHeader) {
	if len(hdrs) == 0 {
		return
	}
	if fv.Type() == fileHdrType {
		fv.Set(reflect.ValueOf(hdrs[0]))
		return
	}
	if fv.Type() == fileHdrSlice {
		fv.Set(reflect.ValueOf(hdrs))
	}
}

func assignField(fv reflect.Value, raw []string) error {
	if fv.Type() == timeType {
		t, err := time.Parse(time.RFC3339, raw[0])
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(t))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw[0])
	case reflect.Bool:
		b, err := strconv.ParseBool(raw[0])
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw[0], 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw[0], 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
	case reflect.Slice:
		if fv.Type().Elem().Kind() == reflect.String {
			out := reflect.MakeSlice(fv.Type(), len(raw), len(raw))
			for i, s := range raw {
				out.Index(i).SetString(s)
			}
			fv.Set(out)
			return nil
		}
		return errors.New("unsupported slice element type")
	default:
		return errors.New("unsupported field type")
	}
	return nil
}
