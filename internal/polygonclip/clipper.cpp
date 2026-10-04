// Keep arc rounding independent of implicit hardware FMA contraction.
#ifdef __clang__
#pragma clang fp contract(off)
#endif
#include "clipper.cpp.inc"
