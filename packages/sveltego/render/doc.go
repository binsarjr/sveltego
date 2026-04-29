// Package render provides a pooled byte buffer used by codegen-emitted SSR
// templates. Generated Render methods take *Writer and stream HTML by
// interleaving WriteString (trusted literals) with WriteEscape (user values).
//
// Typical use inside an HTTP handler:
//
//	w := render.Acquire()
//	defer render.Release(w)
//	if err := page.Render(w, ctx, data); err != nil {
//	    return err
//	}
//	rw.Write(w.Bytes())
package render
