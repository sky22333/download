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
	ASSET_URL      string
	jsdelivrConfig = false

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
		baseURL = "example.com"
		log.Printf("警告: 未设置 GITHUB_URL 环境变量")
	}

	// 移除可能存在的协议前缀和尾部斜杠
	baseURL = strings.TrimPrefix(baseURL, "http://")
	baseURL = strings.TrimPrefix(baseURL, "https://")
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 构建完整的 ASSET_URL
	ASSET_URL = fmt.Sprintf("https://%s/", baseURL)

	// 注册路由处理器
	http.HandleFunc("/gh/", githubProxyHandler)
}

func githubProxyHandler(w http.ResponseWriter, r *http.Request) {
	// 设置CORS头
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

	exp0 := "https:/" + r.Host + "/"
	for strings.Contains(path, exp0) {
		path = strings.Replace(path, exp0, "", 1)
	}

	path = strings.Replace(path, "http:/", "http://", 1)
	path = strings.Replace(path, "https:/", "https://", 1)

	if strings.Contains(path, "githubusercontent.com") {
		httpHandler(w, r, path)
		return
	}

	// 检查URL模式并处理
	if exp1.MatchString(path) || exp3.MatchString(path) || exp4.MatchString(path) ||
		exp5.MatchString(path) || exp6.MatchString(path) || exp7.MatchString(path) ||
		exp8.MatchString(path) || exp9.MatchString(path) {
		httpHandler(w, r, path)
		return
	} else if exp2.MatchString(path) {
		if jsdelivrConfig {
			newURL := strings.Replace(path, "/blob/", "@", 1)
			newURL = regexp.MustCompile(`^(?:https?:\/\/)?github\.com`).ReplaceAllString(newURL, "https://cdn.jsdelivr.net/gh")
			http.Redirect(w, r, newURL, 302)
			return
		} else {
			path = strings.Replace(path, "/blob/", "/raw/", 1)
			httpHandler(w, r, path)
			return
		}
	} else {
		proxyToAsset(w, r, path)
		return
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request, pathname string) {
	if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Headers") != "" {
		setCORSHeaders(w)
		w.WriteHeader(204)
		return
	}

	urlStr := pathname
	if strings.HasPrefix(urlStr, "git") {
		urlStr = "https://" + urlStr
	}

	req, err := http.NewRequest(r.Method, urlStr, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		if key != "Content-Security-Policy" &&
			key != "Content-Security-Policy-Report-Only" &&
			key != "Clear-Site-Data" {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}

	w.Header().Set("Access-Control-Expose-Headers", "*")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

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

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,TRACE,DELETE,HEAD,OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "1728000")
}

func checkUrl(u string) bool {
	patterns := []*regexp.Regexp{exp1, exp2, exp3, exp4, exp5, exp6, exp7, exp8, exp9}
	for _, pattern := range patterns {
		if pattern.MatchString(u) {
			return true
		}
	}
	return false
}

func proxyToAsset(w http.ResponseWriter, r *http.Request, path string) {
	resp, err := http.Get(ASSET_URL + path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
