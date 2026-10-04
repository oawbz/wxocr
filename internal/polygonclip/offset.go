// Package polygonclip statically compiles Clipper 6.4.2 for native-compatible offsets.
package polygonclip

/*
#cgo CXXFLAGS: -std=c++11 -O2
#cgo linux LDFLAGS: -lstdc++ -lm
#cgo windows LDFLAGS: -lstdc++ -lm
#include "bridge.h"
*/
import "C"
import (
	"image"
	"unsafe"
)

func Offset(p []image.Point, distance float64) []image.Point {
	if len(p) < 3 || distance <= 0 {
		return nil
	}
	xy := make([]C.int64_t, 2*len(p))
	for i, v := range p {
		xy[i*2] = C.int64_t(v.X)
		xy[i*2+1] = C.int64_t(v.Y)
	}
	var out *C.int64_t
	var count C.size_t
	if C.wxocr_offset(&xy[0], C.size_t(len(p)), C.double(distance), &out, &count) != 0 {
		return nil
	}
	defer C.wxocr_offset_free(unsafe.Pointer(out))
	data := unsafe.Slice(out, int(count)*2)
	result := make([]image.Point, int(count))
	for i := range result {
		result[i] = image.Pt(int(data[i*2]), int(data[i*2+1]))
	}
	return result
}
