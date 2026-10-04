package ocr

import "testing"

func FuzzJPEGOrientation(f *testing.F) {
	f.Add([]byte{0xff, 0xd8, 0xff, 0xe1, 0, 2, 0xff, 0xd9})
	f.Add([]byte("Exif\x00\x00II\x2a\x00\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, orientation := range []int{jpegOrientation(data), exifOrientation(data)} {
			if orientation < 1 || orientation > 8 {
				t.Fatal("invalid orientation")
			}
		}
	})
}
