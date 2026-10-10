# Linux amd64 Docker 部署

镜像包含程序、网页、模型和 ONNX Runtime；使用镜像内 Debian 12 基础库，无需替换宿主机 libc。宿主机需要能运行 Linux amd64 容器的 Docker Engine 与 Compose 插件。

将整个部署包解压到同一目录，修改 `config.yaml` 中的 Token，然后运行：

```sh
docker load -i wxocr-web-linux-amd64.tar.gz
docker compose up -d
```

访问 `http://服务器IP:7676`，填写配置中的 Token。无需 Go、编译工具链或宿主机 ONNX Runtime。

```sh
docker compose ps
docker compose logs --tail=100
docker compose restart ocr
docker compose down
```

内部监听保持 `0.0.0.0:7676`，模型和运行库路径保持默认。主机端口可用 `OCR_PORT=8080 docker compose up -d` 修改。只允许本机访问可用 `OCR_BIND_ADDRESS=127.0.0.1 docker compose up -d`。配置或二进制更新后执行 `docker compose up -d --force-recreate ocr`，确保重新挂载替换后的文件。

默认日志关闭、debug 关闭、单任务处理，每分钟 60 请求；配置文件只读挂载。容器以前台进程运行，由 Compose 后台启动和自动重启，不使用 start.sh。停止时最多等待 120 秒。内存上限 2 GiB，临时上传文件使用最多 64 MiB 的 /tmp；不适合大量并行任务。

项目 `examples/web/docker` 下的 `Dockerfile` 与 `app/` 为重建材料，使用 `docker build --platform linux/amd64 -t wxocr-web:linux-amd64-20261004-cpu-json .` 重建。常规部署只需要镜像压缩包、compose.yaml 和 config.yaml。

使用 1Panel 等编排面板时，相对路径基于面板的编排目录。请将 Compose 的配置 `source` 改成服务器上实际 YAML 文件的绝对路径，或设置 `OCR_CONFIG_PATH=/opt/app/ocr/config.yaml`。文件必须存在且为普通文件，不能是目录；配置内监听地址需为 `0.0.0.0`。

前端使用 `./api/ocr` 相对路径，不固定目录名。例如页面位于 `/tools/scan/` 时，请求 `/tools/scan/api/ocr`。反向代理需去掉页面目录前缀后转发到容器，并将不带末尾斜线的页面地址重定向到带 `/` 的地址。根目录部署也支持。

程序由部署目录中的 `ocr-web` 只读挂载到 `/app/ocr-web`，网页已嵌入二进制。确保文件存在且可执行：`chmod 755 ocr-web`。在 1Panel 中可设置 `OCR_BINARY_PATH` 为宿主机二进制的绝对路径。

后续同一运行环境下更新程序和网页，只需替换二进制并执行 `docker compose up -d --force-recreate ocr`。建议先将新文件保存为 `ocr-web.new`，赋予执行权限，再通过 `mv ocr-web.new ocr-web` 原子替换；替换后必须重建容器，普通重启可能仍使用旧的文件挂载。保留旧文件可回滚。运行库或系统依赖变化时需要更新对应文件或镜像。

API 只返回通用 OCR JSON（文字、坐标、置信度、段落和耗时）。身份证类型判断与字段提取在 HTML 中完成，“识别类型”标签页位于“纯文字”之前；网页 JSON 与下载结果保留原始 API 数据。
