package ocr

import (
	"bytes"
	"encoding/binary"
	"image"
)

// Only IFD0's orientation is relevant to full-image decoding. Invalid or
// unrelated metadata is ignored, as in the native image reader.
func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 1
	}
	offset := uint64(order.Uint32(tiff[4:8]))
	if offset > uint64(len(tiff)-2) {
		return 1
	}
	count := uint64(order.Uint16(tiff[offset : offset+2]))
	if count > (uint64(len(tiff))-offset-2)/12 {
		return 1
	}
	for i := uint64(0); i < count; i++ {
		entry := tiff[offset+2+i*12 : offset+2+(i+1)*12]
		if order.Uint16(entry[:2]) != 0x112 {
			continue
		}
		if order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 {
			return 1
		}
		value := int(order.Uint16(entry[8:10]))
		if value >= 1 && value <= 8 {
			return value
		}
		return 1
	}
	return 1
}

func jpegOrientation(data []byte) int {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for pos := 2; pos < len(data); {
		// Skip padding and any extraneous bytes before a marker, like libjpeg.
		for pos < len(data) && data[pos] != 0xff {
			pos++
		}
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0 || marker == 0xd8 || marker == 1 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if len(data)-pos < 2 {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if length < 2 || length > len(data)-pos {
			break
		}
		segment := data[pos+2 : pos+length]
		if marker == 0xe1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return exifOrientation(segment[6:])
		}
		pos += length
	}
	return 1
}

func orientJPEG(src *image.NRGBA, orientation int) *image.NRGBA {
	if orientation < 2 || orientation > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	ow, oh := w, h
	if orientation >= 5 {
		ow, oh = h, w
	}
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			sx, sy := x, y
			switch orientation {
			case 2:
				sx = w - 1 - x
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sy = h - 1 - y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			}
			from, to := sy*src.Stride+sx*4, y*out.Stride+x*4
			copy(out.Pix[to:to+4], src.Pix[from:from+4])
		}
	}
	return out
}
