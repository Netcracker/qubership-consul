package main

import (
	"context"
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

	clientset := mustGetKubeClient()

	livePods := getLiveConsulPods(clientset, namespace)

	tokens, err := fetchConsulTokens(consulHost, consulPort, consulToken)
	if err != nil {
		log.Fatalln(err, "Failed fetchConsulTokens")
	}

	tokensByPod := groupTokensByPod(tokens)

	tokensToDelete := pickTokensToDelete(tokensByPod, livePods)

	deleteTokens(consulHost, consulPort, consulToken, tokensToDelete)
}

// ----------------- ENV -----------------

func loadEnv() (host, port, token, ns string) {
	host = strings.TrimSpace(os.Getenv("CONSUL_HOST"))
	port = "8500"
	token = strings.TrimSpace(os.Getenv("CONSUL_HTTP_TOKEN"))
	ns = strings.TrimSpace(os.Getenv("CONSUL_NAMESPACE"))

	if host == "" || token == "" {
		log.Fatal("[ERROR] Missing CONSUL_HOST / CONSUL_HTTP_TOKEN env variables")
	}
	return
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

func fetchConsulTokens(host, port, token string) ([]Token, error) {
	urlStr := fmt.Sprintf("http://%s:%s/v1/acl/tokens", host, port)
	fmt.Println("[INFO] Fetching tokens from Consul...")

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Consul-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

func deleteTokens(host, port, token string, tokensToDelete []string) {
	if len(tokensToDelete) == 0 {
		fmt.Println("[INFO] No stale client tokens to delete.")
		return
	}

	fmt.Println("[INFO] Tokens to delete:")
	for _, id := range tokensToDelete {
		fmt.Println(id)
		idEnc := url.PathEscape(id)
		delURL := fmt.Sprintf("http://%s:%s/v1/acl/token/%s", host, port, idEnc)

		req, err := http.NewRequest("DELETE", delURL, nil)
		if err != nil {
			log.Printf("[WARN] Failed to revoke %s: %v", id, err)
			continue
		}
		req.Header.Set("X-Consul-Token", token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("[WARN] Failed to revoke %s: %v", id, err)
			continue
		}
		resp.Body.Close()

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
