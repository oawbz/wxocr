// Development-only probe for the observed macOS ARM64 WeChat OCR ABI.
// Never link this file or the native engine into the Go runtime.
#include <dlfcn.h>
#include <cstdlib>
#include <iomanip>
#include <iostream>
#include <string>
#include <vector>

struct Point { float x, y; };
struct Size { int width, height; };
struct Text {
    std::string text;
    std::vector<unsigned char> emptyChars; // ABI slot; must remain empty.
    std::vector<Point> polygon;
    float confidence;
};
struct Paragraph { std::vector<Text> lines; std::vector<Point> polygon; };
static_assert(sizeof(Text)==80 && offsetof(Text, polygon)==48);
static_assert(sizeof(Paragraph)==48 && offsetof(Paragraph, polygon)==24);

int main(int argc, char** argv) {
    if (argc != 2) { std::cerr << "usage: paragraph_merge <native-library>\n"; return 2; }
    void* library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
    if (!library) { std::cerr << dlerror() << '\n'; return 1; }
    const char* symbol = "_ZN9wevision222MergeLinesToParagraphsERKNS_4SizeIiEENS_11OrientationERKNSt3__16vectorINS_7OCRTextENS5_9allocatorIS7_EEEERKNS6_INS6_INS_6Point2IfEENS8_ISE_EEEENS8_ISG_EEEERNS6_INS_12OCRParagraphENS8_ISL_EEEE";
    using Merge = void (*)(const Size&, int, const std::vector<Text>&, const std::vector<std::vector<Point>>&, std::vector<Paragraph>&);
    auto merge = reinterpret_cast<Merge>(dlsym(library, symbol));
    if (!merge) { std::cerr << dlerror() << '\n'; return 1; }
    Size size; int direction, count, regions;
    if (!(std::cin >> size.width >> size.height >> direction >> count >> regions) || count<0 || regions<0) return 2;
    std::vector<Text> lines(count);
    for (int i=0;i<count;i++) {
        lines[i].confidence = float(i+1);
        lines[i].polygon.resize(4);
        for (auto& p: lines[i].polygon) if (!(std::cin >> p.x >> p.y)) return 2;
        if (!(std::cin >> std::quoted(lines[i].text))) return 2;
    }
    std::vector<std::vector<Point>> polygons(regions);
    for (auto& polygon: polygons) {
        int n; if (!(std::cin >> n) || n<3) return 2;
        polygon.resize(n);
        for (auto& p:polygon) if (!(std::cin >> p.x >> p.y)) return 2;
    }
    std::vector<Paragraph> paragraphs;
    merge(size, direction, lines, polygons, paragraphs);
    std::cout << "[";
    for (size_t i=0;i<paragraphs.size();i++) {
        if (i) std::cout << ",";
        std::cout << "[";
        for (size_t j=0;j<paragraphs[i].lines.size();j++) {
            if (j) std::cout << ",";
            std::cout << int(paragraphs[i].lines[j].confidence)-1;
        }
        std::cout << "]";
    }
    std::cout << "]\n";
    // Do not dlclose before vectors are destroyed.
}
