package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/cavaliergopher/grab/v3"
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
    "github.com/docker/docker/pkg/jsonmessage"
)

// 下载请求结构体
type DownloadRequest struct {
    Urls []string `json:"urls"`
}

// 文件列表响应结构体
type FileListResponse struct {
    Files []string `json:"files"`
}

// 下载进度结构体
type DownloadProgress struct {
    Index        int     `json:"index"`
    Progress     float64 `json:"progress"`
    IsCompressed bool    `json:"isCompressed"`
    ImageName    string  `json:"imageName"`
}

const downloadDir = "./downloads" // 下载目录

var (
    progressMap       sync.Map
    mutex             sync.Mutex
    compressionStatus sync.Map
)

// 添加 CORS 头
func addCORSHeaders(w http.ResponseWriter, r *http.Request) {
    origin := r.Header.Get("Origin")
    if origin != "" {
        w.Header().Set("Access-Control-Allow-Origin", origin)
    }
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

    if r.Method == http.MethodOptions {
        w.WriteHeader(http.StatusNoContent)
        return
    }
}

// Docker 镜像拉取处理器
func dockerPullHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var req struct {
        Images []string `json:"images"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("无效的请求体: %v", err)
        http.Error(w, "无效的请求体", http.StatusBadRequest)
        return
    }

    go pullPackAndCleanImages(req.Images)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "Docker镜像拉取已开始",
    })
}

// 拉取、打包和清理镜像
func pullPackAndCleanImages(images []string) {
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        log.Printf("创建Docker客户端失败: %v", err)
        return
    }
    defer cli.Close()

    var wg sync.WaitGroup
    for _, image := range images {
        wg.Add(1)
        go func(img string) {
            defer wg.Done()
            pullImage(cli, img)
        }(image)
    }
    wg.Wait()

    // 打包镜像
    tarFileName := fmt.Sprintf("docker_images_%s.tar", time.Now().Format("20060102150405"))
    tarFilePath := filepath.Join(downloadDir, tarFileName)
    if err := packImages(cli, images, tarFilePath); err != nil {
        log.Printf("打包镜像失败: %v", err)
        return
    }

    // 更新压缩状态，仅在成功打包后设置为true
    for _, image := range images {
        compressionStatus.Store(image, true)
    }

	// 清除进度，延迟1秒后
	go func() {
	    time.Sleep(1 * time.Second)
	    for _, image := range images {
	        log.Printf("清除镜像 %s 的下载进度", image)
	        progressMap.Delete(image)
	    }
	}()

    // 清理镜像
    for _, image := range images {
        if _, err := cli.ImageRemove(context.Background(), image, types.ImageRemoveOptions{}); err != nil {
            log.Printf("删除镜像 %s 失败: %v", image, err)
        }
    }

    // 更新所有镜像进度为完成
    for _, image := range images {
        updateDockerProgress(image, 100)
    }

    log.Printf("镜像已拉取，打包到 %s，并清理完毕", tarFilePath)
}

// 拉取单个镜像
func pullImage(cli *client.Client, image string) {
    if !strings.Contains(image, ":") {
        image += ":latest"
    }

    reader, err := cli.ImagePull(context.Background(), image, types.ImagePullOptions{})
    if err != nil {
        log.Printf("拉取镜像 %s 失败: %v", image, err)
        return
    }
    defer reader.Close()

    d := json.NewDecoder(reader)
    for {
        var msg jsonmessage.JSONMessage
        if err := d.Decode(&msg); err != nil {
            if err == io.EOF {
                break
            }
            log.Printf("解码Docker拉取消息时出错: %v", err)
            return
        }
        if msg.Progress != nil && msg.Progress.Total > 0 {
            progress := float64(msg.Progress.Current) / float64(msg.Progress.Total) * 100
            log.Printf("镜像 %s: 进度 %.2f%%", image, progress)
            updateDockerProgress(image, progress)
        }
    }
}

// 更新 Docker 镜像进度
func updateDockerProgress(image string, progress float64) {
    mutex.Lock()
    defer mutex.Unlock()
    progressMap.Store(image, progress)
    // 初始化压缩状态为false
    if _, exists := compressionStatus.Load(image); !exists {
        compressionStatus.Store(image, false)
    }
}

// 打包镜像
func packImages(cli *client.Client, images []string, tarFilePath string) error {
    tarFile, err := os.Create(tarFilePath)
    if err != nil {
        return fmt.Errorf("创建tar文件失败: %v", err)
    }
    defer tarFile.Close()

    for _, image := range images {
        reader, err := cli.ImageSave(context.Background(), []string{image})
        if err != nil {
            return fmt.Errorf("保存镜像 %s 失败: %v", image, err)
        }

        _, err = io.Copy(tarFile, reader)
        reader.Close()
        if err != nil {
            return fmt.Errorf("将镜像 %s 写入tar文件失败: %v", image, err)
        }
    }

    return nil
}

// 文件下载处理器
func downloadHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var req DownloadRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("无效的请求体: %v", err)
        http.Error(w, "无效的请求体", http.StatusBadRequest)
        return
    }

    var wg sync.WaitGroup
    for i, url := range req.Urls {
        wg.Add(1)
        go func(index int, url string) {
            defer wg.Done()
            downloadFile(url, index)
        }(i, url)
    }

	go func() {
	    wg.Wait()
	    log.Println("所有下载已完成")
	    // 更新所有文件进度为完成
	    for i := range req.Urls {
	        updateProgress(i, 100)
	    }
	    time.Sleep(2 * time.Second) // 延迟2秒后清除进度数据
	    clearProgressData()         // 清除进度数据
	}()

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{
        "status": "下载已开始",
    })
}

// 单个文件下载
func downloadFile(url string, index int) {
    client := grab.NewClient()
    client.UserAgent = "non-default-user-agent"
    req, err := grab.NewRequest(downloadDir, url)
    if err != nil {
        log.Printf("创建URL %s 的请求时出错: %v", url, err)
        updateProgress(index, 0)
        return
    }

    resp := client.Do(req)
    t := time.NewTicker(500 * time.Millisecond)
    defer t.Stop()

    for {
        select {
        case <-t.C:
            progress := 100 * resp.Progress()
            updateProgress(index, progress)
            log.Printf("下载进度 %d: %.2f%%\n", index, progress)

        case <-resp.Done:
            if err := resp.Err(); err != nil {
                log.Printf("下载失败: %v\n", err)
                updateProgress(index, 0)
            } else {
                log.Printf("下载完成: %s\n", resp.Filename)
            }
            return
        }
    }
}

// 更新文件下载进度
func updateProgress(index int, progress float64) {
    mutex.Lock()
    defer mutex.Unlock()
    progressMap.Store(index, progress)
}

// 清除进度数据
func clearProgressData() {
    mutex.Lock()
    defer mutex.Unlock()
    progressMap = sync.Map{}    // 清除进度数据
    compressionStatus = sync.Map{} // 清除压缩状态数据
}

// 进度处理器
func progressHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    var progressList []DownloadProgress
    progressMap.Range(func(key, value interface{}) bool {
        var isCompressed bool
        var imageName string
        if status, ok := compressionStatus.Load(key); ok {
            isCompressed = status.(bool)
        }
        switch k := key.(type) {
        case int:
            progressList = append(progressList, DownloadProgress{
                Index:        k,
                Progress:     value.(float64),
                IsCompressed: isCompressed,
            })
        case string:
            imageName = k
            progressList = append(progressList, DownloadProgress{
                Index:        -1, // 用于区分镜像进度
                Progress:     value.(float64),
                IsCompressed: isCompressed,
                ImageName:    imageName,
            })
        }
        return true
    })

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(progressList)
}

// 文件列表处理器
func filesHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    files, err := os.ReadDir(downloadDir)
    if err != nil {
        log.Printf("无法读取文件列表: %v", err)
        http.Error(w, "无法读取文件列表", http.StatusInternalServerError)
        return
    }

    var fileNames []string
    for _, file := range files {
        if !file.IsDir() {
            fileNames = append(fileNames, file.Name())
        }
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(FileListResponse{
        Files: fileNames,
    })
}

// 文件删除处理器
func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    if r.Method != http.MethodDelete {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    fileName := filepath.Base(r.URL.Path)
    filePath := filepath.Join(downloadDir, fileName)

    if err := os.Remove(filePath); err != nil {
        log.Printf("删除文件失败: %v", err)
        http.Error(w, "删除文件失败", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]bool{
        "success": true,
    })
}

// 文件下载处理器
func downloadFileHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    fileName := filepath.Base(r.URL.Path)
    filePath := filepath.Join(downloadDir, fileName)

    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        http.Error(w, "文件不存在", http.StatusNotFound)
        return
    }

    w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
    w.Header().Set("Content-Type", "application/octet-stream")
    http.ServeFile(w, r, filePath)
}

func main() {
    if err := os.MkdirAll(downloadDir, 0755); err != nil {
        log.Fatalf("创建下载目录失败: %v", err)
    }

    http.Handle("/", http.FileServer(http.Dir("./static")))
    http.HandleFunc("/download", downloadHandler)
    http.HandleFunc("/progress", progressHandler)
    http.HandleFunc("/files", filesHandler)
    http.HandleFunc("/delete/", deleteFileHandler)
    http.HandleFunc("/download/", downloadFileHandler)
    http.HandleFunc("/docker-pull", dockerPullHandler)

    log.Println("服务器已启动，端口 8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}