package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	httpTimeout        = 30 * time.Second
	leaderWaitMax      = 3 * time.Minute
	leaderWaitInitial  = 5 * time.Second
	leaderWaitMaxDelay = 30 * time.Second
)

var (
	httpClient = &http.Client{Timeout: httpTimeout}
	baseURL    = "http://localhost:8500"
)

type Token struct {
	AccessorID  string    `json:"AccessorID"`
	Description string    `json:"Description"`
	CreateTime  time.Time `json:"CreateTime"`
}

type TokenDescription struct {
	Component string `json:"component"`
	Pod       string `json:"pod"`
}

func podKey(pod string) string {
	parts := strings.Split(pod, "/")
	return parts[len(parts)-1]
}

func parseDescription(desc string) *TokenDescription {
	if strings.Contains(desc, "Bootstrap Token") || strings.HasPrefix(desc, "Bootstrap") {
		return nil
	}
	i := strings.Index(desc, "{")
	if i == -1 {
		return nil
	}
	jsonPart := desc[i:]

	var td TokenDescription
	if err := json.Unmarshal([]byte(jsonPart), &td); err != nil {
		return nil
	}
	return &td
}

func main() {
	consulHost, consulPort, consulToken, namespace := loadEnv()
	configureHTTP(consulHost, consulPort)

	clientset := mustGetKubeClient()

	if !waitForLeader(consulToken) {
		log.Println("[WARN] Consul leader is not elected, skipping this run")
		return
	}

	livePods := getLiveConsulPods(clientset, namespace)

	tokens, err := fetchConsulTokens(consulToken)
	if err != nil {
		log.Fatalln(err, "Failed fetchConsulTokens")
	}

	tokensByPod := groupTokensByPod(tokens)

	tokensToDelete := pickTokensToDelete(tokensByPod, livePods)

	deleteTokens(consulToken, tokensToDelete)
}

// ----------------- ENV -----------------

func loadEnv() (host, port, token, ns string) {
	host = strings.TrimSpace(os.Getenv("CONSUL_HOST"))
	port = strings.TrimSpace(os.Getenv("CONSUL_PORT"))
	if port == "" {
		port = "8500"
	}
	token = readConsulToken()
	ns = strings.TrimSpace(os.Getenv("CONSUL_NAMESPACE"))

	if host == "" || token == "" {
		log.Fatal("[ERROR] Missing CONSUL_HOST / Consul ACL token (mounted secret file)")
	}
	return
}

// configureHTTP sets baseURL and, when CONSUL_USE_TLS=true, switches to https
// trusting the CA from CONSUL_CACERT_FILE (system roots if it is not set).
func configureHTTP(host, port string) {
	scheme := "http"
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CONSUL_USE_TLS")), "true") {
		scheme = "https"
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if caFile := strings.TrimSpace(os.Getenv("CONSUL_CACERT_FILE")); caFile != "" {
			pem, err := os.ReadFile(caFile)
			if err != nil {
				log.Fatalf("Cannot read CA file %s: %v", caFile, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				log.Fatalf("No valid certificates in CA file %s", caFile)
			}
			tlsCfg.RootCAs = pool
		}
		httpClient.Transport = &http.Transport{TLSClientConfig: tlsCfg}
	}
	baseURL = fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

const removeTokensPodSecretsDir = "/etc/secrets/remove-tokens-pod-secrets"

func readConsulToken() string {
	tokenPath := filepath.Join(removeTokensPodSecretsDir, "CONSUL_HTTP_TOKEN")
	if data, err := os.ReadFile(tokenPath); err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			return token
		}
	}
	return strings.TrimSpace(os.Getenv("CONSUL_HTTP_TOKEN"))
}

// ----------------- K8S -----------------

func mustGetKubeClient() *kubernetes.Clientset {
	clientset, err := getKubeClient()
	if err != nil {
		log.Fatalf("Cannot create k8s client: %v", err)
	}
	return clientset
}

func getLiveConsulPods(clientset *kubernetes.Clientset, namespace string) map[string]struct{} {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=consul,component=client",
	})
	if err != nil {
		log.Fatalf("Cannot list consul client pods: %v", err)
	}

	livePods := make(map[string]struct{})
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodRunning {
			livePods[pod.Name] = struct{}{}
		}
	}
	fmt.Printf("[INFO] Found %d live consul client pods\n", len(livePods))
	return livePods
}

// ----------------- Consul Tokens -----------------

// waitForLeader polls /v1/status/leader with exponential backoff until Consul
// reports a leader or leaderWaitMax elapses.
func waitForLeader(token string) bool {
	urlStr := baseURL + "/v1/status/leader"
	deadline := time.Now().Add(leaderWaitMax)
	delay := leaderWaitInitial

	for {
		if hasLeader(urlStr, token) {
			return true
		}
		if time.Now().Add(delay).After(deadline) {
			return false
		}
		log.Printf("[WARN] No Consul leader yet, retrying in %s", delay)
		time.Sleep(delay)
		delay *= 2
		if delay > leaderWaitMaxDelay {
			delay = leaderWaitMaxDelay
		}
	}
}

func hasLeader(urlStr, token string) bool {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Consul-Token", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return strings.Trim(strings.TrimSpace(string(body)), `"`) != ""
}

func fetchConsulTokens(token string) ([]Token, error) {
	urlStr := baseURL + "/v1/acl/tokens"
	fmt.Println("[INFO] Fetching tokens from Consul...")

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Consul-Token", token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch tokens %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokens []Token
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("[ERROR] Invalid JSON from Consul: %v", err)
	}
	return tokens, nil
}

func groupTokensByPod(tokens []Token) map[string][]Token {
	tokensByPod := make(map[string][]Token)
	for _, t := range tokens {
		td := parseDescription(t.Description)
		if td == nil || td.Component != "client" || td.Pod == "" {
			continue
		}
		podName := podKey(td.Pod)
		tokensByPod[podName] = append(tokensByPod[podName], t)
	}
	return tokensByPod
}

func pickTokensToDelete(tokensByPod map[string][]Token, livePods map[string]struct{}) []string {
	var tokensToDelete []string

	for podName, toks := range tokensByPod {
		if _, alive := livePods[podName]; !alive {
			// pod is dead -> delete all tokens
			for _, t := range toks {
				tokensToDelete = append(tokensToDelete, t.AccessorID)
			}
			continue
		}

		// pod is alive -> keep latest token
		sort.Slice(toks, func(i, j int) bool {
			return toks[i].CreateTime.After(toks[j].CreateTime)
		})
		for i := 1; i < len(toks); i++ {
			tokensToDelete = append(tokensToDelete, toks[i].AccessorID)
		}
	}

	return tokensToDelete
}

func deleteTokens(token string, tokensToDelete []string) {
	if len(tokensToDelete) == 0 {
		fmt.Println("[INFO] No stale client tokens to delete.")
		return
	}

	fmt.Println("[INFO] Tokens to delete:")
	for _, id := range tokensToDelete {
		fmt.Println(id)
		idEnc := url.PathEscape(id)
		delURL := baseURL + "/v1/acl/token/" + idEnc

		req, err := http.NewRequest("DELETE", delURL, nil)
		if err != nil {
			log.Printf("[WARN] Failed to revoke %s: %v", id, err)
			continue
		}
		req.Header.Set("X-Consul-Token", token)

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[WARN] Failed to revoke %s: %v", id, err)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("[WARN] Failed to revoke %s: %s", id, resp.Status)
		}
	}

	fmt.Println("[DONE] Revoked all stale client tokens.")
}

func getKubeClient() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return kubernetes.NewForConfig(cfg)
	}
	kubeconfig := filepath.Join(".", "kubeconfig")
	if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig not found in current directory")
	}
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("cannot load kubeconfig: %v", err)
	}
	return kubernetes.NewForConfig(cfg)
}
