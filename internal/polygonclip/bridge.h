#include <stdint.h>
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
int wxocr_offset(const int64_t *xy,size_t n,double distance,int64_t **output,size_t *count);
void wxocr_offset_free(void *p);
#ifdef __cplusplus
}
#endif
