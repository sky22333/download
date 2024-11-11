## 项目介绍

<div style="text-align: center;">


**完全开源，go实现的简约文件下载器，通过服务器中转下载文件和docker镜像，解决本地网络不通畅的问题**
</div>

**功能特点：**
* **简单易用：** 用户界面简洁直观，操作方便。
* **高效下载：** 多线程下载，加速文件传输，支持所有直链。
* **部署简单：** `docker`一键部署。
* **轻量级：** 打包好的`docker`镜像和二进制文件仅`19MB`左右
* **镜像下载：** 支持下载`docker镜像`和`ghcr镜像`并自动打包为压缩包
* **访问控制：** 通过环境变量设置访问密码

---

### Docker部署

```
docker run -d \
  --name download \
  -p 8080:8080 \
  -e APP_PASSWORD=admin \
  -v ./downloads:/root/downloads \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --restart always \
  ghcr.io/sky22333/download:latest
```
> 默认运行在8080端口，可自行域名反代并开启HTTPS，密码默认`admin`，可在环境变量指定

---




---

### 预览

<img src="https://github.com/user-attachments/assets/39c638b0-2f2e-46ca-9ae0-b8c152c5f222" alt="PC截图" width="600">

---
<img src="https://github.com/user-attachments/assets/3ce12bef-95e0-48b3-8c81-2ea80049f264" alt="手机截图" width="300">



### 说明

- 用户从前端输入链接

- 后端调用下载模块和docker模块

- 下载文件或镜像到服务器`downloads`文件夹

- 文件夹`downloads`内的文件和镜像压缩包显示到前端

- 用户从前端下载`downloads`文件夹内的内容。

- `docker`镜像下载默认从`docker hub`拉取，必须符合格式`用户名/镜像名:标签`，对于官方仓库请用`library`字段替代用户名，拉取完成后自动打包为压缩包，并自动清除镜像，对于压缩包和文件你可以直接在前端界面下载和删除。

- `ghcr`镜像下载示例，`ghcr.io/sky22333/download:v2.2`

- 后端有更详细的日志，菜鸡纯小白练手的项目，发现BUG的话请大佬们帮忙修修，问就是我也不懂，最后请大家点点星星支持一下。




### 免责声明

* 本程序仅供学习了解, 非盈利目的，请勿下载有版权的文件，请勿下载非法文件，下载和使用本项目即默认接受此条款。
* 使用本程序必循遵守部署免责声明。使用本程序必循遵守部署服务器所在地、所在国家和用户所在国家的法律法规, 程序作者不对使用者任何不当行为负责。
