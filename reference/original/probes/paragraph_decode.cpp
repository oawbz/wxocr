// Development-only probe for the observed macOS ARM64 OpenCV/WeChat ABI.
// Feed the exact exported probability map to native DBDecode, without models.
#include <dlfcn.h>
#include <array>
#include <cstdint>
#include <iomanip>
#include <iostream>
#include <vector>

struct Point { float x, y; };
struct alignas(8) Mat { unsigned char storage[96]; };

int main(int argc, char** argv) {
  if (argc != 2) return 2;
  void* library=dlopen(argv[1],RTLD_NOW|RTLD_LOCAL);
  if (!library) {std::cerr<<dlerror()<<'\n';return 1;}
  using Constructor=void(*)(void*,int,int,int,void*,size_t);
  using Destructor=void(*)(void*);
  using Decode=void(*)(void*,const Mat&,const Mat&,std::vector<std::vector<Point>>&);
  auto constructor=reinterpret_cast<Constructor>(dlsym(library,"_ZN2cv3MatC1EiiiPvm"));
  auto destructor=reinterpret_cast<Destructor>(dlsym(library,"_ZN2cv3MatD1Ev"));
  auto decode=reinterpret_cast<Decode>(dlsym(library,"_ZN9wevision221ParagraphDetectionNew8DBDecodeERKN2cv3MatES4_RNSt3__16vectorINS6_INS_6Point2IfEENS5_9allocatorIS8_EEEENS9_ISB_EEEE"));
  if (!constructor || !destructor || !decode) return 1;
  int32_t dimensions[4]; float scale[2];
  std::cin.read(reinterpret_cast<char*>(dimensions),sizeof(dimensions));
  std::cin.read(reinterpret_cast<char*>(scale),sizeof(scale));
  int w=dimensions[0],h=dimensions[1],rw=dimensions[2],rh=dimensions[3];
  if (!std::cin || w<1 || h<1 || w>4096 || h>4096 || rw<1 || rh<1 || rw>w || rh>h) return 2;
  std::vector<float> probability(w*h);
  std::cin.read(reinterpret_cast<char*>(probability.data()),probability.size()*sizeof(float));
  if (!std::cin) return 2;
  std::vector<uint8_t> mask(rw*rh);
  for (int y=0;y<rh;y++) for(int x=0;x<rw;x++) mask[y*rw+x]=probability[y*w+x]>.3f ? 255 : 0;
  Mat scores{},binary{};
  constructor(&scores,rh,rw,5,probability.data(),w*sizeof(float));
  constructor(&binary,rh,rw,0,mask.data(),rw);
  std::vector<std::vector<Point>> polygons;
  std::array<uint8_t,128> unusedThis{};
  decode(unusedThis.data(),scores,binary,polygons);
  destructor(&scores);destructor(&binary);
  std::cout<<std::setprecision(9)<<'[';
  for(size_t i=0;i<polygons.size();i++) {
    if(i) std::cout<<',';
    std::cout<<'[';
    for(size_t j=0;j<polygons[i].size();j++) {
      if(j) std::cout<<',';
      auto p=polygons[i][j];std::cout<<'['<<p.x<<','<<p.y<<']';
    }
    std::cout<<']';
  }
  std::cout<<"]\n";
}
