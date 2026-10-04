// Package jpegcore contains the bundled, deterministic native JPEG decoder.
package jpegcore

/*
#cgo CFLAGS: -I${SRCDIR} -I${SRCDIR}/upstream -O2
#include "decode.h"
*/
import "C"

import (
	"errors"
	"image"
	"runtime"
	"unsafe"
)

// Decode retains libjpeg's normal recovery of corrupt entropy and premature
// EOF, while returning fatal failures as errors. No C pointers escape this call.
func Decode(data []byte) (*image.NRGBA, error) {
	if len(data) == 0 {
		return nil, errors.New("empty JPEG input")
	}
	var width, height C.int
	var message [256]C.char
	// Bound the decoded allocation, including dimensions from corrupt headers.
	pixels := C.wxocr_jpeg_decode((*C.uchar)(unsafe.Pointer(&data[0])), C.size_t(len(data)), 100_000_000, &width, &height, &message[0], C.size_t(len(message)))
	runtime.KeepAlive(data)
	if pixels == nil {
		return nil, errors.New("JPEG decode: " + C.GoString(&message[0]))
	}
	defer C.wxocr_jpeg_free(unsafe.Pointer(pixels))
	w, h := int(width), int(height)
	return &image.NRGBA{Pix: C.GoBytes(unsafe.Pointer(pixels), C.int(w*h*4)), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}
