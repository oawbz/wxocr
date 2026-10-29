#include "resize.h"

static inline void rgb(const unsigned char *p,int format,int32_t *r,int32_t *g,int32_t *b) {
 if (format==1) {*r=*g=*b=p[0];return;}
 if (format==2 && p[3]!=255) {
  unsigned a=p[3];
  if (!a) {*r=*g=*b=0;return;}
  // Match color.NRGBAModel's 16-bit unpremultiplication and uint8 truncation.
  *r=(unsigned char)(((uint32_t)p[0]*65535/a)>>8);
  *g=(unsigned char)(((uint32_t)p[1]*65535/a)>>8);
  *b=(unsigned char)(((uint32_t)p[2]*65535/a)>>8);return;
 }
 *r=p[0];*g=p[1];*b=p[2];
}
static inline void horizontal(const unsigned char *src,size_t stride,int format,
 int y,int w,const wx_axis *xs,int32_t *out) {
 int channels=format==1?1:4;
 const unsigned char *row=src+(size_t)y*stride;
 for (int x=0;x<w;x++) {
  const wx_axis a=xs[x];int32_t r,g,b,rr,gg,bb;
  rgb(row+(size_t)a.a*channels,format,&r,&g,&b);
  rgb(row+(size_t)a.b*channels,format,&rr,&gg,&bb);
  out[3*x]=r*a.wa+rr*a.wb;
  out[3*x+1]=g*a.wa+gg*a.wb;
  out[3*x+2]=b*a.wa+bb*a.wb;
 }
}
void wx_resize(const unsigned char *src,size_t stride,int format,
 int width,int height,int w,int h,const wx_axis *xs,const wx_axis *ys,
 const float *lut,int32_t *storage,float *out) {
 const size_t n=(size_t)width*height;
 if(w!=width || h!=height) {
  for(int c=0;c<3;c++)for(size_t i=0;i<n;i++)out[c*n+i]=lut[c*256];
 }
 int32_t *upper=storage,*lower=storage+3*w;int upper_y=-1,lower_y=-1;
 for(int y=0;y<h;y++) {
  wx_axis a=ys[y];
  if(lower_y==a.a) {int32_t *tmp=upper;upper=lower;lower=tmp;int yy=upper_y;upper_y=lower_y;lower_y=yy;}
  if(upper_y!=a.a) {horizontal(src,stride,format,a.a,w,xs,upper);upper_y=a.a;}
  if(lower_y!=a.b) {horizontal(src,stride,format,a.b,w,xs,lower);lower_y=a.b;}
  for(int c=0;c<3;c++) {
   float *dst=out+c*n+(size_t)y*width;const float *table=lut+c*256;
   for(int x=0;x<w;x++) {
    // All intermediates are nonnegative and bounded below 2^27.
    int32_t v=(((a.wa*(upper[3*x+c]>>4))>>16)+((a.wb*(lower[3*x+c]>>4))>>16)+2)>>2;
    dst[x]=table[v];
   }
  }
 }
}
