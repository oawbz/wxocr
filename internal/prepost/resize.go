// Package prepost provides exact fixed-point native image preprocessing.
package prepost

/*
#cgo CFLAGS: -O3
#include "resize.h"
*/
import "C"

import (
	"image"
	"math"
	"runtime"
	"unsafe"
)

// Resize handles the common decoded image buffers. Unsupported layouts use the
// caller's generic Go path. Parameters and normalization are computed in Go to
// retain the exact rounding of the reference implementation on each platform.
func Resize(src image.Image, width, height, w, h int) ([]float32, bool) {
	var pix []byte
	var stride, format, channels int
	switch im := src.(type) {
	case *image.NRGBA:
		pix, stride, format, channels = im.Pix, im.Stride, 0, 4
	case *image.RGBA:
		pix, stride, format, channels = im.Pix, im.Stride, 2, 4
	case *image.Gray:
		pix, stride, format, channels = im.Pix, im.Stride, 1, 1
	default:
		return nil, false
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || sw > math.MaxInt32 || sh > math.MaxInt32 || sw > len(pix)/channels {
		return nil, false
	}
	rowBytes := sw * channels
	if stride < rowBytes || (sh > 1 && stride > (len(pix)-rowBytes)/(sh-1)) {
		return nil, false
	}
	if width <= 0 || height <= 0 || w <= 0 || h <= 0 || w > width || h > height || width > math.MaxInt32 || height > math.MaxInt32 {
		return nil, false
	}
	axis := func(length, target int) []C.wx_axis {
		out := make([]C.wx_axis, target)
		for x := range out {
			sx := float64(float32((float64(x)+.5)*float64(length)/float64(target) - .5))
			i := int(math.Floor(sx))
			f := sx - float64(i)
			if i < 0 {
				i = 0
				f = 0
			}
			if i >= length-1 {
				i = length - 1
				f = 0
			}
			out[x] = C.wx_axis{a: C.int32_t(i), b: C.int32_t(min(i+1, length-1)), wa: C.int32_t(math.RoundToEven(float64(1-float32(f)) * 2048)), wb: C.int32_t(math.RoundToEven(f * 2048))}
		}
		return out
	}
	if width > int(^uint(0)>>1)/height/3 {
		return nil, false
	}
	xs, ys := axis(sw, w), axis(sh, h)
	var lut [768]float32
	mean := [3]float32{.485, .456, .406}
	std := [3]float32{.229, .224, .225}
	for c := 0; c < 3; c++ {
		for v := 0; v < 256; v++ {
			lut[c*256+v] = (float32(v)/255 - mean[c]) / std[c]
		}
	}
	rows := make([]int32, w*6)
	out := make([]float32, width*height*3)
	C.wx_resize((*C.uchar)(unsafe.Pointer(&pix[0])), C.size_t(stride), C.int(format), C.int(width), C.int(height), C.int(w), C.int(h), &xs[0], &ys[0], (*C.float)(unsafe.Pointer(&lut[0])), (*C.int32_t)(unsafe.Pointer(&rows[0])), (*C.float)(unsafe.Pointer(&out[0])))
	runtime.KeepAlive(src)
	return out, true
}
