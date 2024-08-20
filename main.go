package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"time"
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

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Auto-Generated Cert"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func main() {
	http.HandleFunc("/", handleProxy)

	// Generate a self-signed certificate since the findCertAndKey function is omitted
	tlsCert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatal("Failed to generate self-signed certificate:", err)
	}

	// Configure HTTPS server with the generated certificate
	server := &http.Server{
		Addr: ":8443",
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12, // Ensure a minimum version of TLS
		},
	}

	log.Println("HTTPS proxy server is running on https://localhost:8443")
	log.Fatal(server.ListenAndServeTLS("", ""))
}
