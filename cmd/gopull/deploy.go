package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type DeployServer struct {
	port        int
	secretKey   string
	appDir      string
	sshKeyPath  string
	gitIgnore   []string
}

type GitHubRepository struct {
	SSHURL   string `json:"ssh_url"`
	CloneURL string `json:"clone_url"`
	Private  bool   `json:"private"`
}

type GitHubPayload struct {
	Repository GitHubRepository `json:"repository"`
}

func NewDeployServer(port int, secretKey, appDir, sshKeyPath string, gitIgnore []string) *DeployServer {
	return &DeployServer{
		port:        port,
		secretKey:   secretKey,
		appDir:      appDir,
		sshKeyPath:  sshKeyPath,
		gitIgnore:   gitIgnore,
	}
}
func (s *DeployServer) Start() error {
	appParentDir := s.appDir
	if strings.HasSuffix(appParentDir, "/public") {
		appParentDir = strings.TrimSuffix(appParentDir, "/public")
	}
	
	if err := os.MkdirAll(appParentDir, 0755); err != nil {
		log.Printf("Warning: Failed to create app parent directory: %v", err)
	}
	
	http.HandleFunc("/", s.handleDeployRequest)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
}

// CheckForUpdates performs a git pull rebase operation to check for and apply updates
func (s *DeployServer) CheckForUpdates(repoURL string) error {
	log.Println("Checking for repository updates...")
	return s.performGitPullRebase(repoURL)
}

func isDirectoryEmpty(dir string) (bool, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return true, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	return len(entries) == 0, nil
}

func (s *DeployServer) performGitClone(repoURL string, gitDir string) error {
	log.Printf("Attempting to clone from URL: %s", repoURL)
	
	parentDir := s.appDir
	if strings.Contains(parentDir, "/") {
		parentDir = parentDir[:strings.LastIndex(parentDir, "/")]
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			log.Printf("Warning: Failed to create parent directory: %v", err)
		}
	}
	
	env := os.Environ()
	
	if _, err := os.Stat(s.sshKeyPath); err == nil {
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", s.sshKeyPath))
	}

	var cmd *exec.Cmd
	if s.appDir == "/app/public" && gitDir == "/app/public.git" {
		cmd = exec.Command("git", "clone", "--separate-git-dir="+gitDir, repoURL, s.appDir)
	} else {
		cmd = exec.Command("git", "clone", repoURL, s.appDir)
	}
	
	cmd.Env = env
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	
	log.Printf("Executing git clone to %s", s.appDir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start git clone command: %v", err)
	}
	
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				log.Printf("git stdout: %s", buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()
	
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Printf("git stderr: %s", buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()
	
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}
	
	return nil
}

func (s *DeployServer) getRemoteURL(gitDir string) (string, error) {
	env := os.Environ()
	env = append(env, fmt.Sprintf("GIT_DIR=%s", gitDir))
	
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Env = env
	
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %v", err)
	}
	
	url := string(output)
	url = strings.TrimSpace(url)
	log.Printf("Got remote URL from git config: %s", url)
	
	return url, nil
}

func (s *DeployServer) performGitPullRebase(repoURL string) error {
	log.Printf("performGitPullRebase called with repoURL: %s", repoURL)
	
	if len(s.gitIgnore) > 0 {
		log.Printf("Git ignore patterns: %v", s.gitIgnore)
	}
	
	env := os.Environ()
	
	if _, err := os.Stat(s.sshKeyPath); err == nil {
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", s.sshKeyPath))
	}
	
	var gitDir string
	var needsClone bool
	var existingRepoURL string
	var err error
	
	if s.appDir == "/app/public" {
		gitDir = "/app/public.git"
		
		if err := os.MkdirAll("/app", 0755); err != nil {
			log.Printf("Warning: Failed to create /app directory: %v", err)
		}
		
		_, gitDirErr := os.Stat(gitDir)
		if os.IsNotExist(gitDirErr) {
			appDirEmpty, appDirErr := isDirectoryEmpty(s.appDir)
			if appDirErr != nil {
				log.Printf("App directory check failed: %v", appDirErr)
				needsClone = true
			} else if !appDirEmpty {
				log.Printf("Directory %s exists with files but no git metadata, will initialize git", s.appDir)
			} else {
				log.Printf("Directory %s is empty or doesn't exist, will perform git clone", s.appDir)
				needsClone = true
			}
		} else {
			isEmpty, err := isDirectoryEmpty(gitDir)
			if err != nil {
				log.Printf("Git directory check failed: %v", err)
				needsClone = true
			} else if isEmpty {
				log.Printf("Git directory %s is empty, will perform git clone", gitDir)
				needsClone = true
			} else {
				existingRepoURL, err = s.getRemoteURL(gitDir)
				if err != nil {
					log.Printf("Failed to get remote URL from git config: %v", err)
					if repoURL != "" {
						log.Printf("Using provided repository URL: %s", repoURL)
					} else {
						return fmt.Errorf("failed to get remote URL: %v", err)
					}
				} else {
					log.Printf("Using existing repository URL: %s", existingRepoURL)
					if repoURL == "" {
						repoURL = existingRepoURL
					}
				}
			}
		}
	} else {
		gitDir = fmt.Sprintf("%s/.git", s.appDir)
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			gitDir = fmt.Sprintf("%s.git", s.appDir)
			if _, err := os.Stat(gitDir); os.IsNotExist(err) {
				log.Printf("Git directory not found at %s/.git or %s.git, will perform git clone", s.appDir, s.appDir)
				needsClone = true
			} else {
				existingRepoURL, err = s.getRemoteURL(gitDir)
				if err != nil {
					log.Printf("Failed to get remote URL from git config: %v", err)
					if repoURL != "" {
						log.Printf("Using provided repository URL: %s", repoURL)
					} else {
						return fmt.Errorf("failed to get remote URL: %v", err)
					}
				} else {
					log.Printf("Using existing repository URL: %s", existingRepoURL)
					if repoURL == "" {
						repoURL = existingRepoURL
					}
				}
			}
		} else {
			existingRepoURL, err = s.getRemoteURL(gitDir)
			if err != nil {
				log.Printf("Failed to get remote URL from git config: %v", err)
				if repoURL != "" {
					log.Printf("Using provided repository URL: %s", repoURL)
				} else {
					return fmt.Errorf("failed to get remote URL: %v", err)
				}
			} else {
				log.Printf("Using existing repository URL: %s", existingRepoURL)
				if repoURL == "" {
					repoURL = existingRepoURL
				}
			}
		}
	}
	
	if needsClone {
		if repoURL == "" {
			return fmt.Errorf("cannot clone: repository URL not found")
		}
		return s.performGitClone(repoURL, gitDir)
	}
	
	env = append(env, fmt.Sprintf("GIT_DIR=%s", gitDir))
	env = append(env, fmt.Sprintf("GIT_WORK_TREE=%s", s.appDir))

	// Step 1: git stash -u
	stashCmd := exec.Command("git", "stash", "-u")
	stashCmd.Dir = s.appDir
	stashCmd.Env = env
	
	log.Printf("Executing git stash -u in %s with GIT_DIR=%s", s.appDir, gitDir)
	stashOutput, err := stashCmd.CombinedOutput()
	if err != nil {
		log.Printf("git stash -u output: %s", stashOutput)
		log.Printf("git stash -u warning: %v", err)
		// Continue even if stash fails
	} else {
		log.Printf("git stash -u output: %s", stashOutput)
	}
	
	// Step 2: git pull --rebase with ignore patterns
	pullArgs := []string{"pull", "--rebase"}
	
	if len(s.gitIgnore) > 0 {
		excludePath := filepath.Join(gitDir, "info", "exclude")
		
		infoDir := filepath.Join(gitDir, "info")
		if err := os.MkdirAll(infoDir, 0755); err != nil {
			log.Printf("Warning: Failed to create git info directory: %v", err)
		}
		
		_, err := os.ReadFile(excludePath)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: Failed to read git exclude file: %v", err)
		}
		
		excludeFile, err := os.OpenFile(excludePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("Warning: Failed to open git exclude file: %v", err)
		} else {
			defer excludeFile.Close()

			_, err = excludeFile.WriteString("\n# Added by GoPull GITIGNORE\n")
			if err != nil {
				log.Printf("Warning: Failed to write to git exclude file: %v", err)
			}
			
			for _, pattern := range s.gitIgnore {
				pattern = strings.TrimSpace(pattern)
				if pattern != "" {
					_, err = excludeFile.WriteString(pattern + "\n")
					if err != nil {
						log.Printf("Warning: Failed to write pattern to git exclude file: %v", err)
					}
				}
			}
			
			log.Printf("Added ignore patterns to %s", excludePath)
		}
	}
	
	pullCmd := exec.Command("git", pullArgs...)
	pullCmd.Dir = s.appDir
	pullCmd.Env = env
	
	stdout, err := pullCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderr, err := pullCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	
	log.Printf("Executing git pull rebase in %s with GIT_DIR=%s", s.appDir, gitDir)
	if err := pullCmd.Start(); err != nil {
		return fmt.Errorf("failed to start git pull command: %v", err)
	}
	
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				log.Printf("git stdout: %s", buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()
	
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				log.Printf("git stderr: %s", buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()
	
	if err := pullCmd.Wait(); err != nil {
		return fmt.Errorf("git pull rebase failed: %v", err)
	}
	
	// Step 3: git stash drop
	dropCmd := exec.Command("git", "stash", "drop")
	dropCmd.Dir = s.appDir
	dropCmd.Env = env
	
	log.Printf("Executing git stash drop in %s with GIT_DIR=%s", s.appDir, gitDir)
	dropOutput, err := dropCmd.CombinedOutput()
	if err != nil {
		log.Printf("git stash drop output: %s", dropOutput)
		log.Printf("git stash drop warning: %v", err)
		// Continue even if stash drop fails
	} else {
		log.Printf("git stash drop output: %s", dropOutput)
	}
	
	return nil
}

func (s *DeployServer) handleDeployRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.secretKey != "" {
		apiKey := r.URL.Query().Get("deploy")
		if apiKey == "" {
			http.Error(w, "API key is required", http.StatusBadRequest)
			return
		}

		if apiKey != s.secretKey {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	var repoURL string
	
	var payload GitHubPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Error parsing GitHub webhook payload: %v", err)
	} else {
		log.Printf("Parsed GitHub webhook payload: %+v", payload)
		
		privateKeyExists := false
		if _, err := os.Stat(s.sshKeyPath); err == nil {
			privateKeyExists = true
		}
		
		// Choose URL based on repository privacy and SSH key availability
		if payload.Repository.Private {
			if privateKeyExists {
				// For private repositories with SSH key, use SSH URL
				if payload.Repository.SSHURL != "" {
					repoURL = payload.Repository.SSHURL
					log.Printf("Repository is private, using SSH URL: %s", repoURL)
				}
			} else {
				// For private repositories without SSH key, use Clone URL but warn
				if payload.Repository.CloneURL != "" {
					repoURL = payload.Repository.CloneURL
					log.Printf("WARNING: Repository is private but no SSH key found. Using Clone URL: %s", repoURL)
					log.Printf("This may fail if the repository requires authentication. Set PRIVATE=true to generate SSH keys.")
				}
			}
		} else {
			// For public repositories, use Clone URL
			if payload.Repository.CloneURL != "" {
				repoURL = payload.Repository.CloneURL
				log.Printf("Repository is public, using Clone URL: %s", repoURL)
			}
		}
		
		// Fallback logic
		if repoURL == "" {
			if privateKeyExists && payload.Repository.SSHURL != "" {
				repoURL = payload.Repository.SSHURL
				log.Printf("Falling back to SSH URL: %s", repoURL)
			} else if payload.Repository.CloneURL != "" {
				repoURL = payload.Repository.CloneURL
				log.Printf("Falling back to Clone URL: %s", repoURL)
			}
		}
	}

	log.Println("Received valid deploy request, updating repository...")
	if err := s.performGitPullRebase(repoURL); err != nil {
		log.Printf("Error updating repository: %v", err)
		http.Error(w, fmt.Sprintf("Failed to update repository: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("Repository update completed successfully")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployment successful"))
}