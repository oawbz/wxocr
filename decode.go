package ocr

import (
	"bufio"
	"image"
	_ "image/png"
	"io"

	"wxocr/internal/jpegcore"
)

// DecodeImage decodes an image using the engine's color decoding path. JPEG
// uses the bundled libjpeg decoder, including its recoverable-error handling;
// EXIF orientation is applied before OCR, so result coordinates describe the
// oriented image. PNG retains RGB under alpha, which preprocessing reads
// without compositing.
// JPEG pixel allocations are limited to 100 million pixels.
func DecodeImage(reader io.Reader) (image.Image, error) {
	input := bufio.NewReader(reader)
	magic, _ := input.Peek(2)
	if len(magic) == 2 && magic[0] == 0xff && magic[1] == 0xd8 {
		data, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		src, err := jpegcore.Decode(data)
		if err != nil {
			return nil, err
		}
		return orientJPEG(src, jpegOrientation(data)), nil
	}
	src, _, err := image.Decode(input)
	return src, err
}
