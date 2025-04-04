package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
)

func generateSSHKeys(privateKeyPath, publicKeyPath string) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %v", err)
	}

	privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open private key file: %v", err)
	}
	defer privateKeyFile.Close()

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key: %v", err)
	}

	cmd := exec.Command("ssh-keygen", "-y", "-f", privateKeyPath)
	publicKey, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate public key: %v", err)
	}

	if err := os.WriteFile(publicKeyPath, publicKey, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %v", err)
	}

	return nil
}