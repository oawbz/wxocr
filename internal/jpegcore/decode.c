#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <setjmp.h>
#include "upstream/jpeglib.h"
#include "decode.h"

struct wxocr_error {
  struct jpeg_error_mgr manager;
  jmp_buf jump;
  char message[JMSG_LENGTH_MAX];
};

/* Heap state remains defined after a libjpeg longjmp. */
struct wxocr_decoder {
  struct jpeg_decompress_struct decoder;
  struct wxocr_error error;
  unsigned char *pixels;
};

static void fail(j_common_ptr decoder) {
  struct wxocr_error *error = (struct wxocr_error *)decoder->err;
  (*decoder->err->format_message)(decoder, error->message);
  longjmp(error->jump, 1);
}

static void quiet_message(j_common_ptr decoder, int level) {
  /* Recoverable entropy/EOF warnings use libjpeg's normal recovery path. */
  if (level < 0) decoder->err->num_warnings++;
}

static void cleanup(struct wxocr_decoder *state) {
  jpeg_destroy_decompress(&state->decoder);
  free(state->pixels);
  free(state);
}

unsigned char *wxocr_jpeg_decode(const unsigned char *input, size_t length,
                                size_t max_pixels, int *width, int *height,
                                char *error, size_t error_length) {
  struct wxocr_decoder *state = calloc(1, sizeof(*state));
  if (!state) {
    snprintf(error, error_length, "JPEG decoder allocation failed");
    return NULL;
  }
  state->decoder.err = jpeg_std_error(&state->error.manager);
  state->error.manager.error_exit = fail;
  state->error.manager.emit_message = quiet_message;
  if (setjmp(state->error.jump)) {
    snprintf(error, error_length, "%s", state->error.message);
    cleanup(state);
    return NULL;
  }
  jpeg_create_decompress(&state->decoder);
  jpeg_mem_src(&state->decoder, input, (unsigned long)length);
  jpeg_read_header(&state->decoder, TRUE);
  size_t w = state->decoder.image_width, h = state->decoder.image_height;
  if (!w || !h || w > max_pixels / h || w > ((size_t)-1) / 4 / h) {
    snprintf(error, error_length, "JPEG dimensions exceed pixel limit");
    cleanup(state);
    return NULL;
  }
  if (state->decoder.data_precision != 8) {
    snprintf(error, error_length, "unsupported JPEG precision: %d",
             state->decoder.data_precision);
    cleanup(state);
    return NULL;
  }
  int cmyk = state->decoder.jpeg_color_space == JCS_CMYK ||
             state->decoder.jpeg_color_space == JCS_YCCK;
  state->decoder.out_color_space = cmyk ? JCS_CMYK : JCS_EXT_RGBA;
  state->decoder.dct_method = JDCT_ISLOW;
  state->decoder.do_fancy_upsampling = TRUE;
  jpeg_start_decompress(&state->decoder);
  state->pixels = malloc(w * h * 4);
  if (!state->pixels) {
    snprintf(error, error_length, "JPEG pixel allocation failed");
    cleanup(state);
    return NULL;
  }
  while (state->decoder.output_scanline < state->decoder.output_height) {
    unsigned char *row = state->pixels + state->decoder.output_scanline * w * 4;
    if (jpeg_read_scanlines(&state->decoder, &row, 1) != 1) {
      snprintf(error, error_length, "JPEG scanline decoding stopped");
      cleanup(state);
      return NULL;
    }
    if (cmyk) {
      /* Match OpenCV's CMYK to RGB integer conversion. */
      for (size_t x = 0; x < w; x++, row += 4) {
        int k = row[3];
        row[0] = k - (((255 - row[0]) * k) >> 8);
        row[1] = k - (((255 - row[1]) * k) >> 8);
        row[2] = k - (((255 - row[2]) * k) >> 8);
        row[3] = 255;
      }
    }
  }
  jpeg_finish_decompress(&state->decoder);
  unsigned char *pixels = state->pixels;
  state->pixels = NULL;
  *width = (int)w;
  *height = (int)h;
  cleanup(state);
  return pixels;
}

void wxocr_jpeg_free(void *buffer) { free(buffer); }
