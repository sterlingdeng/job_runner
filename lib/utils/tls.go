package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func GetCertsFromPath(certPath, keyPath, caPath string) (cert, key, ca []byte, err error) {
	cert, err = os.ReadFile(certPath)
	if err != nil {
		return nil, nil, nil, err
	}
	key, err = os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, nil, err
	}
	ca, err = os.ReadFile(caPath)
	if err != nil {
		return nil, nil, nil, err
	}
	return
}

func GetTlsConfig(caCert, certFile, key []byte) (*tls.Config, error) {
	rootCAs := x509.NewCertPool()
	ok := rootCAs.AppendCertsFromPEM(caCert)
	if !ok {
		return nil, fmt.Errorf("failed to add ca cert to certpool")
	}
	cert, err := tls.X509KeyPair(certFile, key)
	if err != nil {
		return nil, fmt.Errorf("LoadX509KeyPair: %w", err)
	}
	cfg := tls.Config{
		RootCAs:      rootCAs,
		ClientCAs:    rootCAs,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384},
	}
	return &cfg, nil
}
