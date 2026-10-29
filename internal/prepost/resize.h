#ifndef WX_OCR_RESIZE_H
#define WX_OCR_RESIZE_H
#include <stdint.h>
#include <stddef.h>
typedef struct { int32_t a,b,wa,wb; } wx_axis;
void wx_resize(const unsigned char *src, size_t stride, int format,
 int width,int height,int w,int h,const wx_axis *xs,const wx_axis *ys,
 const float *lut,int32_t *storage,float *out);
#endif
