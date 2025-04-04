package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	var (
		port       = flag.Int("port", 15800, "Port to listen for deploy requests")
		appDir     = flag.String("app-dir", "/app/public", "Directory to perform git pull rebase")
		secretKey  = flag.String("secret-key", "", "Secret key for API validation")
		sshKeyPath = flag.String("ssh-key-path", "/keys", "Path to store SSH keys")
	)
	flag.Parse()

	envSecretKey := os.Getenv("DEPLOY_KEY")
	if envSecretKey != "" {
		secretKey = &envSecretKey
	}

	gitRepoURL := os.Getenv("GIT_REPOURL")
	
	gitIgnoreStr := os.Getenv("GITIGNORE")
	var gitIgnore []string
	if gitIgnoreStr != "" {
		gitIgnore = strings.Split(gitIgnoreStr, ",")
		log.Printf("Git ignore patterns: %v", gitIgnore)
	}
	
	privateMode := false
	envPrivate := os.Getenv("PRIVATE")
	if envPrivate == "true" {
		privateMode = true
		log.Println("Running in PRIVATE mode, SSH keys will be generated")
		
		// In PRIVATE mode, DEPLOY_KEY is required
		if *secretKey == "" {
			log.Fatal("Secret key is required in PRIVATE mode. Set it via DEPLOY_KEY environment variable or --secret-key flag")
		}
	} else {
		log.Println("Running in non-PRIVATE mode, SSH keys will not be generated")
		
		// In non-PRIVATE mode, GIT_REPOURL is required
		if gitRepoURL == "" {
			log.Fatal("GIT_REPOURL is required in non-PRIVATE mode. Set it via GIT_REPOURL environment variable")
		}
		
		// In non-PRIVATE mode, DEPLOY_KEY is optional
		if *secretKey == "" {
			log.Println("No DEPLOY_KEY provided. Webhook functionality will be disabled.")
		}
	}

	if err := os.MkdirAll(*sshKeyPath, 0700); err != nil {
		log.Fatalf("Failed to create SSH key directory: %v", err)
	}

	privateKeyPath := filepath.Join(*sshKeyPath, "id_rsa")
	publicKeyPath := filepath.Join(*sshKeyPath, "id_rsa.pub")

	// Generate SSH keys only if PRIVATE mode is enabled and keys don't exist
	if privateMode {
		if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
			log.Println("Generating new SSH keys...")
			if err := generateSSHKeys(privateKeyPath, publicKeyPath); err != nil {
				log.Fatalf("Failed to generate SSH keys: %v", err)
			}
			
			if err := os.Chmod(privateKeyPath, 0600); err != nil {
				log.Fatalf("Failed to set permissions on private key: %v", err)
			}
			
			// Display the public key for GitHub Deploy Key setup
			publicKey, err := os.ReadFile(publicKeyPath)
			if err != nil {
				log.Fatalf("Failed to read public key: %v", err)
			}
			fmt.Println("=== GitHub Deploy Key (Add this to your GitHub repository) ===")
			fmt.Println(string(publicKey))
			fmt.Println("============================================================")
			
			if err := os.Remove(publicKeyPath); err != nil {
				log.Printf("Warning: Failed to remove public key file: %v", err)
			} else {
				log.Println("Public key file removed after display")
			}
		}
	}

	log.Printf("Starting GoPull deploy server on port %d", *port)
	log.Printf("Watching for changes in %s", *appDir)
	
	server := NewDeployServer(*port, *secretKey, *appDir, privateKeyPath, gitIgnore)
	
	// In non-PRIVATE mode, set up automatic repository updates
	if !privateMode && gitRepoURL != "" {
		log.Printf("Setting up automatic updates for repository: %s", gitRepoURL)
		
		// Start a goroutine to check for updates every minute
		go func() {
			for {
				time.Sleep(1 * time.Minute)
				log.Println("Checking for repository updates...")
				if err := server.CheckForUpdates(gitRepoURL); err != nil {
					log.Printf("Error checking for updates: %v", err)
				} else {
					log.Println("Repository update check completed")
				}
			}
		}()
	}
	
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}