package images

import "image"

// scaleBilinear returns a new RGBA image that is src downscaled to
// (dstW, dstH) using bilinear interpolation. The implementation is
// stdlib-only so the package keeps Go 1.23 vanilla; for v1's
// build-time-only resampling at 320/640/1280-wide targets, bilinear
// quality is well above acceptable.
//
// Upscaling is allowed but the caller is expected to gate it (see
// processSource which skips widths >= the intrinsic width).
func scaleBilinear(src image.Image, dstW, dstH int) image.Image {
	if dstW <= 0 || dstH <= 0 {
		return src
	}
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	// xRatio/yRatio map the destination pixel center back into the source
	// coordinate space. Subtracting one from the source extents and the
	// destination extents keeps the corner samples aligned to the corner
	// pixels rather than off-grid by half a pixel.
	xRatio := float64(srcW-1) / float64(dstW)
	yRatio := float64(srcH-1) / float64(dstH)

	for y := 0; y < dstH; y++ {
		fy := float64(y) * yRatio
		y0 := int(fy)
		y1 := y0 + 1
		if y1 >= srcH {
			y1 = srcH - 1
		}
		dy := fy - float64(y0)
		for x := 0; x < dstW; x++ {
			fx := float64(x) * xRatio
			x0 := int(fx)
			x1 := x0 + 1
			if x1 >= srcW {
				x1 = srcW - 1
			}
			dx := fx - float64(x0)

			c00r, c00g, c00b, c00a := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y0).RGBA()
			c10r, c10g, c10b, c10a := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y0).RGBA()
			c01r, c01g, c01b, c01a := src.At(srcBounds.Min.X+x0, srcBounds.Min.Y+y1).RGBA()
			c11r, c11g, c11b, c11a := src.At(srcBounds.Min.X+x1, srcBounds.Min.Y+y1).RGBA()

			r := bilerp(c00r, c10r, c01r, c11r, dx, dy)
			g := bilerp(c00g, c10g, c01g, c11g, dx, dy)
			b := bilerp(c00b, c10b, c01b, c11b, dx, dy)
			a := bilerp(c00a, c10a, c01a, c11a, dx, dy)

			off := dst.PixOffset(x, y)
			dst.Pix[off+0] = uint8(r >> 8)
			dst.Pix[off+1] = uint8(g >> 8)
			dst.Pix[off+2] = uint8(b >> 8)
			dst.Pix[off+3] = uint8(a >> 8)
		}
	}
	return dst
}

// bilerp performs bilinear interpolation between four 16-bit channel
// samples, with weights dx (column) and dy (row), and returns the
// interpolated 16-bit value. Operating on the full 16-bit channel keeps
// resampling precision higher than collapsing to 8 bits up front.
func bilerp(c00, c10, c01, c11 uint32, dx, dy float64) uint32 {
	top := float64(c00)*(1-dx) + float64(c10)*dx
	bot := float64(c01)*(1-dx) + float64(c11)*dx
	return uint32(top*(1-dy) + bot*dy + 0.5)
}
