# 使用更小的 Node.js Alpine 镜像
FROM node:14-alpine

# 设置工作目录
WORKDIR /usr/src/app

# 安装 wget 和其他必要的工具
RUN apk add --no-cache wget

# 复制 package.json 和 package-lock.json
COPY package*.json ./

# 安装项目依赖
RUN npm install --production && \
    npm cache clean --force  # 清理 npm 缓存以减小镜像体积

# 复制项目文件
COPY . .

# 创建下载目录
RUN mkdir -p downloads

# 暴露端口
EXPOSE 3000

# 启动应用
CMD ["node", "app.js"]
