#!/bin/bash
# 一键安装脚本

# 定义颜色
GREEN='\033[1;32m'
RED='\033[1;31m'
NC='\033[0m' # 无颜色

# 定义变量
DOWNLOAD_URL="https://github.com/sky22333/download/releases/download/v2.0/download-linux64.zip"
INSTALL_DIR="/usr/local"
SERVICE_NAME="download.service"
EXECUTABLE_NAME="download-linux64"

# 日志函数
log_success() {
    echo -e "${GREEN}$1${NC}"
}

log_error() {
    echo -e "${RED}$1${NC}" >&2
}

# 下载文件
echo "开始下载最新版本..."
if ! wget -q -O /tmp/download-linux64.zip "${DOWNLOAD_URL}"; then
    log_error "下载失败，请检查链接和网络。"
    exit 1
fi

# 检查下载是否成功
if [[ ! -f /tmp/download-linux64.zip ]]; then
    log_error "下载文件不存在，请检查链接。"
    exit 1
fi

# 解压文件
echo "正在解压..."
if ! unzip -o /tmp/download-linux64.zip -d "${INSTALL_DIR}" > /dev/null; then
    log_error "解压失败，请检查文件。"
    exit 1
fi

rm -f /tmp/download-linux64.zip

# 增加执行权限
echo "设置执行权限..."
if ! chmod +x "${INSTALL_DIR}/${EXECUTABLE_NAME}"; then
    log_error "设置执行权限失败。"
    exit 1
fi

# 创建 systemd 服务文件
echo "创建服务文件..."
sudo tee /etc/systemd/system/${SERVICE_NAME} > /dev/null <<EOF
[Unit]
Description=Download Service

[Service]
ExecStart=${INSTALL_DIR}/${EXECUTABLE_NAME}
WorkingDirectory=${INSTALL_DIR}
Restart=always
User=root

[Install]
WantedBy=multi-user.target
EOF


if [[ $? -ne 0 ]]; then
    log_error "创建服务文件失败。"
    exit 1
fi

if ! sudo systemctl daemon-reload; then
    log_error "重新加载服务配置失败。"
    exit 1
fi

if ! sudo systemctl start ${SERVICE_NAME} || ! sudo systemctl enable ${SERVICE_NAME}; then
    log_error "启动服务失败。"
    exit 1
fi

# 提示安装成功
log_success "安装成功，服务已启动，运行在8080端口。"