package main

import (
    "encoding/json"
    "github.com/cavaliergopher/grab/v3"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type DownloadRequest struct {
    Urls []string `json:"urls"`
}

type FileListResponse struct {
    Files []string `json:"files"`
}

type DownloadProgress struct {
    Index    int     `json:"index"`
    Progress float64 `json:"progress"`
}

const downloadDir = "./downloads"

var (
    progressMap sync.Map
    mutex       sync.Mutex
)

func addCORSHeaders(w http.ResponseWriter, r *http.Request) {
    origin := r.Header.Get("Origin")
    if origin != "" {
        w.Header().Set("Access-Control-Allow-Origin", origin)
    }
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
    
    if r.Method == http.MethodOptions {
        return
    }
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var req DownloadRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        log.Printf("Invalid request body: %v", err)
        http.Error(w, "Invalid request body", http.StatusBadRequest)
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
        log.Println("All downloads completed")
        time.Sleep(2 * time.Second) // 延迟2秒后清除进度数据
        clearProgressData()          // 清除进度数据
    }()

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{
        "status": "Downloads started",
    })
}

func downloadFile(url string, index int) {
    client := grab.NewClient()
    // 设置自定义用户代理 https://github.com/cavaliergopher/grab/issues/104
    client.UserAgent = "non-default-user-agent"
    req, err := grab.NewRequest(downloadDir, url)
    if err != nil {
        log.Printf("Error creating request for URL %s: %v", url, err)
        updateProgress(index, 0) // 更新为0%进度
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
                updateProgress(index, 100)
            }
            return
        }
    }
}

func updateProgress(index int, progress float64) {
    mutex.Lock()
    defer mutex.Unlock()
    progressMap.Store(index, progress)
}

func clearProgressData() {
    mutex.Lock()
    defer mutex.Unlock()
    progressMap = sync.Map{} // 清除进度数据
}

func progressHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)

    var progressList []DownloadProgress
    progressMap.Range(func(key, value interface{}) bool {
        progressList = append(progressList, DownloadProgress{
            Index:    key.(int),
            Progress: value.(float64),
        })
        return true
    })

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(progressList)
}

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

    log.Println("服务器已启动，端口 8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
