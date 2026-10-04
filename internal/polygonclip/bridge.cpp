#include "bridge.h"
#include "clipper.hpp"
#include <cstdlib>
#include <cmath>
int wxocr_offset(const int64_t *xy,size_t n,double distance,int64_t **output,size_t *count) {
 *output=nullptr;*count=0;
 try {
  ClipperLib::Path input;input.reserve(n);
  for(size_t i=0;i<n;i++)input.emplace_back(xy[i*2],xy[i*2+1]);
  ClipperLib::ClipperOffset offset;offset.AddPath(input,ClipperLib::jtRound,ClipperLib::etClosedPolygon);
  ClipperLib::Paths paths;offset.Execute(paths,distance);
  if(paths.empty())return 0;
  size_t best=0;for(size_t i=1;i<paths.size();i++)if(std::fabs(ClipperLib::Area(paths[i]))>std::fabs(ClipperLib::Area(paths[best])))best=i;
  const auto &p=paths[best];auto *result=static_cast<int64_t*>(std::malloc(p.size()*2*sizeof(int64_t)));
  if(!result && !p.empty())return 1;
  for(size_t i=0;i<p.size();i++){result[2*i]=p[i].X;result[2*i+1]=p[i].Y;}
  *output=result;*count=p.size();return 0;
 }catch(...){return 1;}
}
void wxocr_offset_free(void *p){std::free(p);}
