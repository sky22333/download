package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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

type DownloadRequest struct {
	Urls []string `json:"urls"`
}

type FileListResponse struct {
	Files []string `json:"files"`
}

type DownloadProgress struct {
	Index        int     `json:"index"`
	Progress     float64 `json:"progress"`
	IsCompressed bool    `json:"isCompressed"`
	ImageName    string  `json:"imageName"`
}

type Session struct {
	Key       string
	CreatedAt time.Time
}

const (
	downloadDir = "./downloads"
	keyLength   = 32
)

var (
	progressMap       sync.Map
	mutex             sync.Mutex
	compressionStatus sync.Map
	sessions          sync.Map
	sessionDuration   = 8 * time.Hour
)

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

func generateSessionKey() (string, error) {
	bytes := make([]byte, keyLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func validateSession(key string) bool {
	if value, ok := sessions.Load(key); ok {
		session := value.(Session)
		if time.Since(session.CreatedAt) < sessionDuration {
			return true
		}
		sessions.Delete(key)
	}
	return false
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			addCORSHeaders(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		addCORSHeaders(w, r)

		if r.URL.Path == "/login" {
			next(w, r)
			return
		}

		sessionKey := r.Header.Get("X-Session-Key")
		if !validateSession(sessionKey) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w, r)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	expectedPassword := os.Getenv("APP_PASSWORD")
	if expectedPassword == "" {
		log.Fatal("环境变量未设置")
	}

	if req.Password != expectedPassword {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	sessionKey, err := generateSessionKey()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sessions.Store(sessionKey, Session{
		Key:       sessionKey,
		CreatedAt: time.Now(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"sessionKey": sessionKey,
	})
}

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

	// 从镜像名称中提取信息并生成文件名
	var imageInfos []string
	for _, image := range images {
		processedName := strings.ReplaceAll(image, "/", "_")
		processedName = strings.ReplaceAll(processedName, ":", "_")

		if !strings.Contains(image, ":") {
			processedName = processedName + "_latest"
		}

		imageInfos = append(imageInfos, fmt.Sprintf("docker_%s", processedName))
	}

	tarFileName := fmt.Sprintf("%s.tar", strings.Join(imageInfos, "_"))
	tarFilePath := filepath.Join(downloadDir, tarFileName)
	if err := packImages(cli, images, tarFilePath); err != nil {
		log.Printf("打包镜像失败: %v", err)
		return
	}

	for _, image := range images {
		compressionStatus.Store(image, true)
	}

	go func() {
		time.Sleep(2 * time.Second)
		for _, image := range images {
			log.Printf("清除镜像 %s 的下载进度", image)
			progressMap.Delete(image)
		}
	}()

	for _, image := range images {
		if _, err := cli.ImageRemove(context.Background(), image, types.ImageRemoveOptions{}); err != nil {
			log.Printf("删除镜像 %s 失败: %v", image, err)
		}
	}

	for _, image := range images {
		updateDockerProgress(image, 100)
	}

	log.Printf("镜像已拉取，打包到 %s，并清理完毕", tarFilePath)
}

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

func updateDockerProgress(image string, progress float64) {
	mutex.Lock()
	defer mutex.Unlock()
	progressMap.Store(image, progress)
	if _, exists := compressionStatus.Load(image); !exists {
		compressionStatus.Store(image, false)
	}
}

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
		for i := range req.Urls {
			updateProgress(i, 100)
		}
		time.Sleep(2 * time.Second)
		clearProgressData()
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "下载已开始",
	})
}

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

func updateProgress(index int, progress float64) {
	mutex.Lock()
	defer mutex.Unlock()
	progressMap.Store(index, progress)
}

func clearProgressData() {
	mutex.Lock()
	defer mutex.Unlock()
	progressMap = sync.Map{}
	compressionStatus = sync.Map{}
}

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
				Index:        -1,
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
	if os.Getenv("APP_PASSWORD") == "" {
		log.Fatal("环境变量未设置")
	}

	http.Handle("/", http.FileServer(http.Dir("./public")))
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/download", authMiddleware(downloadHandler))
	http.HandleFunc("/progress", authMiddleware(progressHandler))
	http.HandleFunc("/files", authMiddleware(filesHandler))
	http.HandleFunc("/delete/", deleteFileHandler)
	http.HandleFunc("/download/", downloadFileHandler)
	http.HandleFunc("/docker-pull", authMiddleware(dockerPullHandler))

	go func() {
		for {
			time.Sleep(time.Hour)
			sessions.Range(func(key, value interface{}) bool {
				session := value.(Session)
				if time.Since(session.CreatedAt) >= sessionDuration {
					sessions.Delete(key)
				}
				return true
			})
		}
	}()

	log.Println("服务器已启动，面板端口 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
