## 项目介绍

<div style="text-align: center;">


**练手项目，go实现的简约文件下载器网页版。**
</div>

**功能特点：**
* **简单易用：** 用户界面简洁直观，操作方便。
* **高效下载：** 支持多线程下载，加速文件传输。
* **跨平台：** 支持`docker`

**1**：用户从前端输入链接

**2**：后端调用`cavaliergopher/grab/v3`模块

**3**：下载文件到服务器`downloads`文件夹

**4**：服务器`downloads`文件内容渲染到前端

**5**：用户从服务器`downloads`文件内下载指定文件到本地

---

### Docker部署

```
git clone https://github.com/sky22333/download.git
```

```
cd download
```
```
docker compose up -d
```
**项目默认运行在`8080`端口**

---

### 预览

<img src="https://github.com/user-attachments/assets/db9329dc-4b83-4fa2-9648-2e8c7f909d7b" alt="PC截图" width="600">

<img src="https://github.com/user-attachments/assets/0e02a2cc-541a-4a45-8a53-6bbfd20a6d40" alt="PC截图" width="600">

---
<img src="https://github.com/user-attachments/assets/f478386c-54ef-48d9-b56d-ce9bb22746f6" alt="手机截图" width="300">

