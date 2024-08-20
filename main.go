package main

import (
    "io/ioutil"
    "net/http"
    "os"
)

const (
    upstream        = "https://login.microsoftonline.com"
    upstreamPath    = "/"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // Default to port 8080 if not set
    }

    http.HandleFunc("/", handleRequest)
    if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
        os.Exit(1)
    }
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
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

    // Copy headers from the original request
    copyHeaders(r.Header, req.Header)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        http.Error(w, "Error contacting upstream server", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)

    // Handle response headers and CORS
    modifyHeadersForCORS(resp.Header, w.Header())

    w.WriteHeader(resp.StatusCode)
    w.Write(body)
}

func handlePreflight(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    w.WriteHeader(http.StatusOK)
}

func modifyHeadersForCORS(src http.Header, dst http.Header) {
    for key, values := range src {
        for _, value := range values {
            dst.Add(key, value)
        }
    }
    // Add CORS headers
    dst.Set("Access-Control-Allow-Origin", "*")
    dst.Set("Access-Control-Allow-Credentials", "true")
}

func copyHeaders(src http.Header, dst http.Header) {
    for k, v := range src {
        for _, vv := range v {
            dst.Add(k, vv)
        }
    }
}
