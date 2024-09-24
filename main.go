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

func addCORSHeaders(w http.ResponseWriter) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w)

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
    }()

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{
        "status": "Downloads started",
    })
}

func downloadFile(url string, index int) {
    client := grab.NewClient()
    req, err := grab.NewRequest(downloadDir, url)
    if err != nil {
        log.Printf("Error creating request for URL %s: %v", url, err)
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

func progressHandler(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w)

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
    addCORSHeaders(w)

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
    addCORSHeaders(w)

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

func main() {
    if err := os.MkdirAll(downloadDir, 0755); err != nil {
        log.Fatalf("创建下载目录失败: %v", err)
    }

    http.Handle("/", http.FileServer(http.Dir("./static")))
    http.HandleFunc("/download", downloadHandler)
    http.HandleFunc("/progress", progressHandler)
    http.HandleFunc("/files", filesHandler)
    http.HandleFunc("/delete/", deleteFileHandler)

    log.Println("服务器已启动，端口 8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}