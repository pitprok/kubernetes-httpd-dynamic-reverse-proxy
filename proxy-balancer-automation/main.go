package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"time"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var config Config

//Config - Used to import configuration from config.yml
type Config struct {
	HttpdPodName       string            `yaml:"httpdPodName"`
	HttpdContainerName string            `yaml:"httpdContainerName"`
	HttpdBinary        string            `yaml:"httpdBinary"`
	ProxyBalancerConf  string            `yaml:"proxyBalancerConf"`
	TomcatImagePattern string            `yaml:"tomcatImage"`
	TomcatLabels       map[string]string `yaml:"tomcatLabels"`
}

// Imports configuration from config.yml and stores it in a global variable
func importUserConfiguration() {
	filename, _ := filepath.Abs("./config.yml")
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
}

func containerIsActive(pod *v1.Pod, containerStatus v1.ContainerStatus) bool {
	return containerStatus != v1.ContainerStatus{} &&
		containerStatus.Ready == true &&
		containerStatus.State.Running != nil &&
		pod.Status.Conditions[1].Status == "True" && /* (Pod)Ready */
		pod.Status.Conditions[2].Status == "True" && /* ContainersReady */
		pod.ObjectMeta.DeletionGracePeriodSeconds == nil &&
		pod.ObjectMeta.DeletionTimestamp == nil
}

//ipAlreadyExists checks if the IP of a tomcat container already exists
//in the configuration of mod_proxy_balancer
func ipAlreadyExists(podIP string, containerPort string) bool {
	grepRegex := fmt.Sprintf("%s:%s", podIP, containerPort)

	cmd := exec.Command("kubectl", "exec", config.HttpdPodName, "--",
		"grep", grepRegex, config.ProxyBalancerConf)

	err := cmd.Run()
	if err != nil {
		if err.Error() == "exit status 1" {
			return false
		}
		panic(err.Error())
	}
	return true
}

//addToBalancer adds the ip and port of a tomcat server
//to the configuration of mod_proxy_balancer
//after checking if it isn't already included
//and reloads httpd's configuration
func addToBalancer(podIP string, containerPort string) {
	if !ipAlreadyExists(podIP, containerPort) {
		fmt.Println("Adding " + podIP + ":" + containerPort + " to " + config.ProxyBalancerConf)
		sedExpression := "s|\\(<Proxy \"balancer:.*>\\)|\\1\\n    BalancerMember \"http://" + podIP + ":" + containerPort + "\"|"

		cmd := exec.Command("kubectl", "exec", config.HttpdPodName, "--",
			"sed", "-i", sedExpression, config.ProxyBalancerConf)

		err := cmd.Run()
		if err != nil {
			panic(err.Error())
		}
		reloadHttpdConfig()
	}

}

//removeFromBalancer removes the ip and port of a tomcat server
//from the configuration of mod_proxy_balancer
//and reloads httpd's configuration
func removeFromBalancer(podIP string, containerPort string) {
	fmt.Println("Removing " + podIP + ":" + containerPort + " from " + config.ProxyBalancerConf)
	sedExpression := "/    BalancerMember \"http:\\/\\/" + podIP + ":" + containerPort + "\"/d"
	cmd := exec.Command("kubectl", "exec", config.HttpdPodName, "--",
		"sed", "-i", sedExpression, config.ProxyBalancerConf)
	err := cmd.Run()
	if err != nil {
		panic(err.Error())
	}
	reloadHttpdConfig()
}

func reloadHttpdConfig() {
	fmt.Println("Reloading httpd configuration.")
	cmd := exec.Command("kubectl", "exec", "httpd", "--container", config.HttpdContainerName, "--",
		config.HttpdBinary, "-k", "graceful")
	err := cmd.Run()
	if err != nil {
		panic(err.Error())
	}
}

func patternMatch(regex string, str string) bool {
	regex = fmt.Sprintf("^%s$", regex)
	matched, err := regexp.MatchString(regex, str)
	if err != nil {
		panic(err.Error())
	}
	return matched
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func podContainsHttpd(pod *v1.Pod) bool {
	return (pod.ObjectMeta.Name == config.HttpdPodName)
}

func getHttpdContainerStatus(pod *v1.Pod) v1.ContainerStatus {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == config.HttpdContainerName {
			return containerStatus
		}
	}
	return v1.ContainerStatus{}
}

func getHttpdContainerSpecs(pod *v1.Pod) v1.Container {
	for _, containerSpecs := range pod.Spec.Containers {
		if containerSpecs.Name == config.HttpdContainerName {
			return containerSpecs
		}
	}
	return v1.Container{}
}

//addMissingBalancerMembers tries to add all the online tomcat servers
//which fit the criteria set by the user to the configuration of mod_proxy_balancer
func addMissingBalancerMembers(tomcats map[string]string) {
	if len(tomcats) > 0 {
		fmt.Println("Adding missing balancer members")
	}
	for tomcatIP, tomcatPort := range tomcats {
		addToBalancer(tomcatIP, tomcatPort)
	}

}

func handleHttpdPod(pod *v1.Pod, tomcats map[string]string, httpdOnline bool) bool {
	podIP := pod.Status.PodIP

	if podIP != "" &&
		len(pod.Status.Conditions) > 0 &&
		len(pod.Status.ContainerStatuses) > 0 {

		httpdStatus := getHttpdContainerStatus(pod)
		containerIsActive := containerIsActive(pod, httpdStatus)

		if containerIsActive {

			addMissingBalancerMembers(tomcats)

			httpdOnline = true

		} else if pod.ObjectMeta.DeletionGracePeriodSeconds != nil ||
			pod.ObjectMeta.DeletionTimestamp != nil {

			httpdOnline = false

		} else {
			httpdOnline = false
		}
	}
	return httpdOnline
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func podContainsTomcat(pod *v1.Pod) bool {
	imagesMatch := false
	labelsMatch := false
	for _, container := range pod.Spec.Containers {
		imagesMatch = patternMatch(config.TomcatImagePattern, container.Image)
		if imagesMatch {
			break
		}
	}
	if len(config.TomcatLabels) == 0 {
		labelsMatch = true
	} else {
		for tomcatLabelKey, tomcatLabelVal := range config.TomcatLabels {
			labelsMatch = false
			for podLabelKey, podLabelVal := range pod.ObjectMeta.Labels {
				if podLabelKey == tomcatLabelKey &&
					podLabelVal == tomcatLabelVal {
					labelsMatch = true
					break
				}
			}
			if !labelsMatch {
				break
			}
		}
	}
	if imagesMatch &&
		labelsMatch {
		return true
	}

	return false
}

func getTomcatContainerStatus(pod *v1.Pod) v1.ContainerStatus {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if patternMatch(config.TomcatImagePattern, containerStatus.Image) {
			return containerStatus
		}
	}
	return v1.ContainerStatus{}
}

func getTomcatContainerSpecs(pod *v1.Pod, tomcatStatus v1.ContainerStatus) v1.Container {
	for _, containerSpecs := range pod.Spec.Containers {
		if containerSpecs.Image == tomcatStatus.Image {
			return containerSpecs
		}
	}
	return v1.Container{}
}

//tomcatResponds tries to send a get request to the tomcat server,
//retrying after 1 second if it there is no response,
//up to 10 times
func tomcatResponds(podIP string, tomcatPort string) bool {
	for i := 0; i < 10; i++ {
		resp, err := http.Get("http://" + podIP + ":" + tomcatPort)
		if err != nil {
			panic(err.Error())
		}
		if resp.StatusCode == 200 {
			return true
		}
		time.Sleep(1000 * time.Millisecond)
	}
	return false
}

func handleTomcatPod(pod *v1.Pod, tomcats map[string]string, httpdOnline bool) {
	podIP := pod.Status.PodIP
	podName := pod.ObjectMeta.Name
	if podIP != "" &&
		len(pod.Status.Conditions) > 0 &&
		len(pod.Status.ContainerStatuses) > 0 {

		tomcatStatus := getTomcatContainerStatus(pod)
		tomcatSpecs := getTomcatContainerSpecs(pod, tomcatStatus)
		containerIsActive := containerIsActive(pod, tomcatStatus)

		// We assume that only one port is exposed on the tomcat container
		containerPort := strconv.Itoa(int(tomcatSpecs.Ports[0].ContainerPort))

		_, ok := tomcats[podIP]
		if ok &&
			!containerIsActive {
			if httpdOnline {
				removeFromBalancer(podIP, containerPort)
			}
			delete(tomcats, podIP)
		} else if !ok &&
			containerIsActive {
			if tomcatResponds(podIP, containerPort) {
				if httpdOnline {
					addToBalancer(podIP, containerPort)
				} else {
					fmt.Println("Tomcat server in pod \"" + podName + "\" is online, but an instance of httpd can't be found")
					fmt.Println("The tomcat server will be added to httpd's balancer members when an active httpd is detected")
				}
				tomcats[podIP] = containerPort
			}
		}
	}
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func main() {
	importUserConfiguration()
	tomcats := make(map[string]string)
	httpdOnline := false

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	api := clientset.CoreV1()
	listOptions := metav1.ListOptions{}

	watcher, err := api.Pods("").Watch(context.TODO(), listOptions)
	if err != nil {
		panic(err.Error())
	}
	ch := watcher.ResultChan()
	for event := range ch {
		pod, ok := event.Object.(*v1.Pod)

		if !ok {
			panic("Unexpected type")
		}

		if podContainsHttpd(pod) {
			httpdOnline = handleHttpdPod(pod, tomcats, httpdOnline)
		}
		if podContainsTomcat(pod) {
			handleTomcatPod(pod, tomcats, httpdOnline)
		}

	}
}
