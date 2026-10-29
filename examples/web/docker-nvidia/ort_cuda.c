// Enable CUDA on sessions created by the Go binding. Provider errors fail startup.
// The CPU package and its ONNX Runtime library are unaffected.
#include "onnxruntime_c_api.h"
#include <dlfcn.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdatomic.h>

static const OrtApi *real_api;
static OrtApi gpu_api;
static OrtApiBase gpu_base;
static void *runtime_handle;
static pthread_once_t once = PTHREAD_ONCE_INIT;
static atomic_ulong profile_sequence;

static OrtStatus *ORT_API_CALL gpu_session(const OrtEnv *env, const ORTCHAR_T *path,
    const OrtSessionOptions *input_options, OrtSession **out) {
  OrtSessionOptions *options = NULL;
  OrtStatus *status = input_options ? real_api->CloneSessionOptions(input_options, &options)
                                   : real_api->CreateSessionOptions(&options);
  if (status) return status;
  const char *name = strrchr(path, '/'); name = name ? name + 1 : path;
  int use_gpu = 1;
  OrtCUDAProviderOptions provider = {0};
  provider.device_id = 0;
  provider.gpu_mem_limit = (size_t)2 * 1024 * 1024 * 1024;
  provider.cudnn_conv_algo_search = OrtCudnnConvAlgoSearchHeuristic;
  provider.do_copy_in_default_stream = 1;
  status = real_api->SessionOptionsAppendExecutionProvider_CUDA(options, &provider);
  if (!status && getenv("WXOCR_GPU_PROFILE")) {
    char profile[256];
    snprintf(profile, sizeof(profile), "/app/cache/ort-%s-%s-%lu", use_gpu ? "gpu" : "cpu", name, atomic_fetch_add(&profile_sequence, 1));
    status = real_api->EnableProfiling(options, profile);
  }
  if (!status) {
    if (getenv("WXOCR_GPU_VERBOSE")) fprintf(stderr, "wxocr: %s provider for %s, FP32\n", "CUDA", name);
    status = real_api->CreateSession(env, path, options, out);
  }
  real_api->ReleaseSessionOptions(options);
  return status;
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
  gpu_api.CreateSession = gpu_session;
  gpu_base.GetApi = get_api;
  gpu_base.GetVersionString = base->GetVersionString;
}
ORT_EXPORT const OrtApiBase *ORT_API_CALL OrtGetApiBase(void) {
  pthread_once(&once,initialize);
  return real_api ? &gpu_base : NULL;
}
