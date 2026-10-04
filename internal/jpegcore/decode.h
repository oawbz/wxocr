#ifndef WXOCR_JPEG_DECODE_H
#define WXOCR_JPEG_DECODE_H
#include <stddef.h>

/* Returns an owned RGBA buffer; free with wxocr_jpeg_free(). */
unsigned char *wxocr_jpeg_decode(const unsigned char *input, size_t length,
                                size_t max_pixels, int *width, int *height,
                                char *error, size_t error_length);
void wxocr_jpeg_free(void *buffer);
#endif
