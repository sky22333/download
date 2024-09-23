const express = require('express');
const bodyParser = require('body-parser');
const { exec } = require('child_process');
const fs = require('fs');
const path = require('path');
const app = express();

app.use(bodyParser.json());

// 提供静态文件（如index.html）
app.use(express.static(path.join(__dirname, 'public')));

// 处理文件下载请求
app.post('/download', (req, res) => {
    const url = req.body.url;
    if (!url) {
        return res.status(400).json({ error: '请提供下载链接' });
    }

    const fileName = path.basename(url);
    const filePath = path.join(__dirname, 'downloads', fileName);

    const wgetCommand = `wget -O ${filePath} ${url}`;
    exec(wgetCommand, (error, stdout, stderr) => {
        if (error) {
            console.error(`下载出错: ${error.message}`);
            return res.status(500).json({ error: '下载失败' });
        }

        console.log(`文件下载成功: ${filePath}`);
        res.json({ filePath: `/downloads/${fileName}` });
    });
});

// 提供下载的文件
app.use('/downloads', express.static(path.join(__dirname, 'downloads')));

// 获取下载目录中的文件列表
app.get('/files', (req, res) => {
    const downloadsDir = path.join(__dirname, 'downloads');
    fs.readdir(downloadsDir, (err, files) => {
        if (err) {
            return res.status(500).json({ error: '无法读取文件列表' });
        }
        res.json({ files });
    });
});

// 删除文件
app.delete('/delete/:fileName', (req, res) => {
    const fileName = req.params.fileName;
    const filePath = path.join(__dirname, 'downloads', fileName);

    fs.unlink(filePath, (err) => {
        if (err) {
            return res.status(500).json({ error: '删除文件失败' });
        }
        res.json({ success: true });
    });
});

// 启动服务器
app.listen(3000, () => {
    console.log('服务器运行在端口 3000');
});
