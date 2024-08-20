package main

import (
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

func handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL, err := url.Parse("https://login.microsoftonline.com")
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	// Create the proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers from the original request to the proxy request
	for name, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Create a custom HTTP client with TLS configuration
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Don't verify the certificate (not recommended for production)
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: transport,
	}

	// Perform the request
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Failed to perform request to target server", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy the headers from the response
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Copy the status code from the response
	w.WriteHeader(resp.StatusCode)

	// Copy the body from the response
	io.Copy(w, resp.Body)
}

func findCertAndKey() (string, string, error) {
	// Specify the directory to search for SSL certificate and key files
	certDir := "./certs"

	// Define the expected certificate and key filenames
	var certFile, keyFile string

	// Walk through the directory to find the cert and key files
	err := filepath.Walk(certDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if filepath.Ext(path) == ".crt" {
				certFile = path
			} else if filepath.Ext(path) == ".key" {
				keyFile = path
			}
		}
		return nil
	})

	if err != nil {
		return "", "", err
	}

	// Check if both files were found
	if certFile == "" || keyFile == "" {
		return "", "", os.ErrNotExist
	}

	return certFile, keyFile, nil
}

func main() {
	http.HandleFunc("/", handleProxy)

	// Attempt to find the SSL certificate and key files
	certFile, keyFile, err := findCertAndKey()
	if err != nil {
		log.Fatal("Failed to find SSL certificate and key files:", err)
	}

	// Configure HTTPS server
	server := &http.Server{
		Addr: ":8443",
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, // Ensure a minimum version of TLS
		},
	}

	log.Println("HTTPS proxy server is running on https://localhost:8443")
	log.Fatal(server.ListenAndServeTLS(certFile, keyFile))
}
