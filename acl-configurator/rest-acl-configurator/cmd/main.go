// Copyright 2024-2025 NetCracker Technology Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var deploymentRes = schema.GroupVersionResource{
	Group:    getEnv("API_GROUP", "netcracker.com"),
	Version:  "v1alpha1",
	Resource: "consulacls"}

var kubeconfig = new(string)

var allowNamespaces = prepareAllowNamespacesList()

func prepareAllowNamespacesList() []string {
	nameSpacesString := os.Getenv("ALLOWED_NAMESPACES")
	return strings.Split(nameSpacesString, ",")
}

func getKubeConfig() *string {
	if *kubeconfig != "" {
		return kubeconfig
	}
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	return kubeconfig
}

func getConfigurationForKubernetesClient() *rest.Config {
	var config *rest.Config
	var err error
	isClusterConfig := os.Getenv("IS_CLUSTER_CONFIG")
	if isClusterConfig == "" || isClusterConfig == "true" {
		config, err = rest.InClusterConfig()
	} else {
		kubeconfig := getKubeConfig()
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}
	if err != nil {
		log.Fatalln(err, "Can not get kubernetes config")
		return nil
	}
	return config
}

func makeClient() dynamic.Interface {
	config := getConfigurationForKubernetesClient()
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalln(err, "Can not get dynamic kubernetes client")
	}
	return client
}

func makeKubeClient() *kubernetes.Clientset {
	config := getConfigurationForKubernetesClient()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln(err, "Can not get kubernetes client")
	}
	return clientset
}

func main() {
	log.Fatal(http.ListenAndServe(
		fmt.Sprintf("%s:%s", "localhost", "8088"),
		AdapterMainHandler(),
	))
}

func AdapterMainHandler() http.Handler {
	r := mux.NewRouter()
	r.Handle("/reconcile",
		handlers.LoggingHandler(
			os.Stdout,
			http.HandlerFunc(handlerFunc),
		)).Methods("GET")

	return JsonContentType(handlers.CompressHandler(r))
}

func handlerFunc(w http.ResponseWriter, r *http.Request) {
	if authenticated := checkAuth(extractToken(r)); !authenticated {
		_, err := w.Write([]byte("Access denied"))
		if err != nil {
			log.Printf("Can not write authentication response due to: %v", err)
		}
		w.WriteHeader(401)
		return
	}
	if ok := changeReconcileSpecField(); !ok {
		_, err := w.Write([]byte("Some errors occurred during reconciliation"))
		if err != nil {
			log.Printf("Can not write failed reconciliation response due to: %v", err)
		}
		w.WriteHeader(504)
	} else {
		_, err := w.Write([]byte("Reconciliation done!"))
		if err != nil {
			log.Printf("Can not write successful reconciliation response due to: %v", err)
		}
		w.WriteHeader(200)
	}
}

func extractToken(r *http.Request) string {
	bearerToken := r.Header.Get("Authorization")
	strArr := strings.Split(bearerToken, " ")
	if len(strArr) == 2 {
		return strArr[1]
	}
	return ""
}

func JsonContentType(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { // error handler, when error occurred it sends request with http status 400 and body with error message
			if err := recover(); err != nil {
				log.Printf("Exception: %v\n", err)
				http.Error(w, err.(string), http.StatusBadRequest)
				return
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
	})
}

func changeReconcileSpecField() bool {
	client := makeClient()
	result := true
	message := getCommonReconcileMessage()
	log.Printf("Start reconciliation proccess with id %s", message)
	objs, err := client.Resource(deploymentRes).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Println("Can not get list of consulacls custom resources")
		return false
	}
	for _, obj := range objs.Items {
		aclSpecField, ok := (obj.Object["spec"]).(map[string]interface{})["acl"]
		if !ok {
			result = false
			log.Printf("Error: Can not convert acl spec field to map[string]interface{} type for object -%s ", obj)
			continue
		}
		aclSpec, ok := (aclSpecField).(map[string]interface{})
		if !ok {
			result = false
			log.Printf("Error: Can not convert acl.name spec field to map[string]interface{} type for object - %s", obj)
			continue
		}
		aclSpec["commonReconcile"] = message
		_, err := client.Resource(deploymentRes).Update(context.TODO(), &obj, metav1.UpdateOptions{})
		if err != nil {
			result = false
			log.Printf("Error: Can't update consulacls custom resorce - %s", obj)
			continue
		}
	}
	return result
}

func getCommonReconcileMessage() string {
	return fmt.Sprintf("Common reconciliation at %s-%s",
		strconv.Itoa(time.Now().Second()),
		strconv.Itoa(time.Now().Nanosecond()))
}

func checkAuth(token string) bool {
	clientSet := makeKubeClient()
	reviewer := clientSet.AuthenticationV1().TokenReviews()
	tokenReview := v1.TokenReview{}
	tokenReview.Spec.Token = token
	reviewResult, err := reviewer.Create(context.TODO(), &tokenReview, metav1.CreateOptions{})
	if err != nil {
		log.Println("can not check service account token")
		return false
	} else {
		authenticated := reviewResult.Status.Authenticated
		if authenticated {
			authenticated = checkAuthorization(token)
		}
		return authenticated
	}
}

func checkAuthorization(tokenString string) bool {
	if len(allowNamespaces) == 1 && allowNamespaces[0] == "" {
		return true
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(""), nil
	})
	if err != nil {
		log.Printf("Can not parse token string due to: %v", err)
	}
	namespace := claims["kubernetes.io/serviceaccount/namespace"]
	return isNamespaceAllowed(namespace.(string))
}

func isNamespaceAllowed(namespace string) bool {
	for _, item := range allowNamespaces {
		if namespace == item {
			return true
		}
	}
	return false
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
