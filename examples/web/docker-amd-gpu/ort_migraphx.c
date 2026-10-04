// Experimental C API adapter: enable MIGraphX on sessions created by the Go binding.
// The CPU package and its ONNX Runtime library are unaffected.
#include "onnxruntime_c_api.h"
#include <dlfcn.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static const OrtApi *real_api;
static OrtApi gpu_api;
static OrtApiBase gpu_base;
static void *runtime_handle;
static pthread_once_t once = PTHREAD_ONCE_INIT;

static OrtStatus *ORT_API_CALL gpu_options(OrtSessionOptions **out) {
  OrtStatus *status = real_api->CreateSessionOptions(out);
  if (status) return status;
  OrtMIGraphXProviderOptions provider = {0};
  status = real_api->SessionOptionsAppendExecutionProvider_MIGraphX(*out, &provider);
  if (!status && getenv("WXOCR_GPU_PROFILE"))
    status = real_api->EnableProfiling(*out, "/app/cache/ort-gpu");
  if (status) { real_api->ReleaseSessionOptions(*out); *out = NULL; return status; }
  fprintf(stderr, "wxocr: MIGraphX provider enabled, device=0, FP32\n");
  return NULL;
}
static const OrtApi *ORT_API_CALL get_api(uint32_t version) {
  return version == ORT_API_VERSION && real_api ? &gpu_api : NULL;
}
static void initialize(void) {
  const char *path = getenv("WXOCR_ORT_REAL_LIBRARY");
  if (!path) path = "/opt/ort/libonnxruntime.so.1.19.0";
  runtime_handle = dlopen(path, RTLD_NOW | RTLD_LOCAL);
  if (!runtime_handle) { fprintf(stderr,"wxocr: GPU runtime load failed: %s\n",dlerror()); return; }
  const OrtApiBase *(*get_base)(void) = dlsym(runtime_handle,"OrtGetApiBase");
  if (!get_base) { fprintf(stderr,"wxocr: missing OrtGetApiBase\n"); return; }
  const OrtApiBase *base = get_base();
  real_api = base->GetApi(ORT_API_VERSION);
  if (!real_api) { fprintf(stderr,"wxocr: GPU runtime C API version mismatch\n"); return; }
  memcpy(&gpu_api,real_api,sizeof(gpu_api));
  gpu_api.CreateSessionOptions = gpu_options;
  gpu_base.GetApi = get_api;
  gpu_base.GetVersionString = base->GetVersionString;
}
ORT_EXPORT const OrtApiBase *ORT_API_CALL OrtGetApiBase(void) {
  pthread_once(&once,initialize);
  return real_api ? &gpu_base : NULL;
}
