package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
)

const (
    upstreamURL = "https://login.microsoftonline.com" // The upstream API URL
)

func main() {
    // Get the port from environment variables or default to 8080
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // Define API routes
    http.HandleFunc("/api/request", handleRequest)
    http.HandleFunc("/api/preflight", handlePreflight)

    // Start the server
    fmt.Printf("Starting server on port %s...\n", port)
    if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}

// handleRequest handles the incoming requests from the frontend and forwards them to the upstream API
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // Parse the incoming request body
    var requestBody map[string]interface{}
    err := json.NewDecoder(r.Body).Decode(&requestBody)
    if err != nil {
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }

    // Prepare the request to the upstream service
    req, err := http.NewRequest("POST", upstreamURL+"/common/GetCredentialType?mkt=en-US", nil)
    if err != nil {
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // Forward necessary headers
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", r.Header.Get("Authorization"))

    // Make the request to the upstream service
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        http.Error(w, "Upstream request failed", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Read the response from the upstream service
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        http.Error(w, "Failed to read upstream response", http.StatusInternalServerError)
        return
    }

    // Forward the response back to the frontend
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(resp.StatusCode)
    w.Write(body)
}

// handlePreflight handles CORS preflight requests
func handlePreflight(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    w.WriteHeader(http.StatusOK)
}
