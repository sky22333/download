package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const (
	PREFIX = "/gh/"
)

var (
	jsdelivrConfig = false
	ASSET_URL      string

	// GitHub 相关正则表达式
	exp1 = regexp.MustCompile(`^(?:https?:\/\/)?github\.com\/.+?\/.+?\/(?:releases|archive)\/.*$`)
	exp2 = regexp.MustCompile(`^(?:https?:\/\/)?github\.com\/.+?\/.+?\/(?:blob|raw)\/.*$`)
	exp3 = regexp.MustCompile(`^(?:https?:\/\/)?github\.com\/.+?\/.+?\/(?:info|git-).*$`)
	exp4 = regexp.MustCompile(`^(?:https?:\/\/)?raw\.(?:githubusercontent|github)\.com\/.+?\/.+?\/.+?\/.+$`)
	exp5 = regexp.MustCompile(`^(?:https?:\/\/)?gist\.(?:githubusercontent|github)\.com\/.+?\/.+?\/.+$`)
	exp6 = regexp.MustCompile(`^(?:https?:\/\/)?github\.com\/.+?\/.+?\/tags.*$`)
	exp7 = regexp.MustCompile(`^(?:https?:\/\/)?api\.github\.com\/.*$`)
	exp8 = regexp.MustCompile(`^(?:https?:\/\/)?git\.io\/.*$`)
	exp9 = regexp.MustCompile(`^(?:https?:\/\/)?gitlab\.com\/.*$`)

	whiteList = []string{} // 白名单路径
)

func init() {
	// 从环境变量获取 GitHub URL
	baseURL := os.Getenv("GITHUB_URL")
	if baseURL == "" {
		// 设置默认值
		baseURL = "example.com"
		log.Printf("警告: 未设置 GITHUB_URL 环境变量")
	}

	// 移除可能存在的协议前缀和尾部斜杠
	baseURL = strings.TrimPrefix(baseURL, "http://")
	baseURL = strings.TrimPrefix(baseURL, "https://")
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建完整的 ASSET_URL
	ASSET_URL = fmt.Sprintf("https://%s/", baseURL)

	// 注册 /gh/ 路由处理器
	http.HandleFunc("/gh/", githubProxyHandler)
}

func githubProxyHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	path := r.URL.Path[len(PREFIX):]

	if q := r.URL.Query().Get("q"); q != "" {
		http.Redirect(w, r, "https://"+r.Host+PREFIX+q, 301)
		return
	}

	if path == "perl-pe-para" {
		handlePerlPara(w, r)
		return
	}

	if strings.Contains(path, r.Host+"/gh/") {
		path = strings.Replace(path, r.Host+"/gh/", "", 1)
	}

	if strings.HasPrefix(path, "https:/") && !strings.HasPrefix(path, "https://") {
		path = "https://" + path[7:]
	}
	if strings.HasPrefix(path, "http:/") && !strings.HasPrefix(path, "http://") {
		path = "http://" + path[6:]
	}

	if matchGitHubPatterns(path) {
		proxyGitHubRequest(w, r, path)
		return
	}

	proxyToAsset(w, r, path)
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,TRACE,DELETE,HEAD,OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "1728000")
}

func matchGitHubPatterns(path string) bool {
	patterns := []*regexp.Regexp{exp1, exp2, exp3, exp4, exp5, exp6, exp7, exp8, exp9}
	for _, pattern := range patterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

func proxyGitHubRequest(w http.ResponseWriter, r *http.Request, path string) {
	if strings.HasPrefix(path, "https:/") && !strings.HasPrefix(path, "https://") {
		path = "https://" + path[7:]
	}
	if strings.HasPrefix(path, "http:/") && !strings.HasPrefix(path, "http://") {
		path = "http://" + path[6:]
	}

	if exp2.MatchString(path) && !jsdelivrConfig {
		path = strings.Replace(path, "/blob/", "/raw/", 1)
	}

	req, err := http.NewRequest(r.Method, path, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	copyHeaders(req.Header, r.Header)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	handleRedirect(w, resp)

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handlePerlPara(w http.ResponseWriter, r *http.Request) {
	perlstr := "perl -pe"
	responseText := fmt.Sprintf(`s#(bash.*?\.sh)([^/\w\d])#\1 | %s "\$(curl -L %s/gh/perl-pe-para)" \2#g; s# (git)# https://\1#g; s#(http.*?git[^/]*?/)#%s/gh/\1#g`,
		perlstr, r.Host, r.Host)

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "max-age=300")
	w.Write([]byte(responseText))
}

func proxyToAsset(w http.ResponseWriter, r *http.Request, path string) {
	assetURL := ASSET_URL + path
	resp, err := http.Get(assetURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func handleRedirect(w http.ResponseWriter, resp *http.Response) {
	if location := resp.Header.Get("Location"); location != "" {
		if matchGitHubPatterns(location) {
			if strings.Contains(location, "/gh/") {
				location = strings.Replace(location, "/gh/", "", 1)
			}
			w.Header().Set("Location", PREFIX+location)
		} else {
			w.Header().Set("Location", location)
		}
	}
}
