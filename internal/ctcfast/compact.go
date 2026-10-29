// Package ctcfast reduces logits to exact greedy CTC evidence without retaining
// a full normalized class matrix. Double accumulation matches the Go reference.
package ctcfast

/*
#cgo CFLAGS: -O3
#cgo LDFLAGS: -lm
#include <math.h>
#include <stdlib.h>
static int compact_ctc(const float *data,int steps,int classes,float *out) {
 int *ids=(int*)malloc(sizeof(int)*steps);
 double *maximum=(double*)malloc(sizeof(double)*steps);
 double *denominator=(double*)malloc(sizeof(double)*steps);
 double *exponents=(double*)malloc(sizeof(double)*classes);
 if(!ids||!maximum||!denominator||!exponents) {free(ids);free(maximum);free(denominator);free(exponents);return 2;}
 for(int t=0;t<steps;t++) {
  const float *row=data+(size_t)t*classes;
  int best=0;
  for(int c=0;c<classes;c++) {if(!isfinite(row[c])){free(ids);free(maximum);free(denominator);free(exponents);return 1;}if(row[c]>row[best])best=c;}
  double max=(double)row[best],sum=0;
  for(int c=0;c<classes;c++) {exponents[c]=exp((double)row[c]-max);sum+=exponents[c];}
  // Match the reference's float32 probability ties, including unequal logits
  // that round to the same normalized probability. Lowest class index wins.
  float best_probability=(float)(1.0/sum);
  for(int c=0;c<best;c++) {if((float)(exponents[c]/sum)==best_probability){best=c;break;}}
  ids[t]=best;maximum[t]=max;denominator[t]=sum;
 }
 for(int t=0;t<steps;t++) {
  const float *row=data+(size_t)t*classes;
  float *dst=out+(size_t)t*(steps+2);
  double max=maximum[t],sum=denominator[t];
  dst[0]=(float)ids[t];dst[1]=(float)(exp((double)row[0]-max)/sum);
  for(int s=0;s<steps;s++)dst[2+s]=(float)(exp((double)row[ids[s]]-max)/sum);
 }
 free(ids);free(maximum);free(denominator);free(exponents);return 0;
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

func Compact(data []float32, steps, classes int) ([]float32, error) {
	if steps < 1 || steps > 4096 || classes < 1 || len(data)/steps != classes || len(data)%steps != 0 {
		return nil, errors.New("invalid recognition logits shape")
	}
	out := make([]float32, steps*(steps+2))
	status := C.compact_ctc((*C.float)(unsafe.Pointer(&data[0])), C.int(steps), C.int(classes), (*C.float)(unsafe.Pointer(&out[0])))
	if status != 0 {
		return nil, errors.New("non-finite logits or CTC allocation failure")
	}
	return out, nil
}
