package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	// "time"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

var httpdPodName = "httpd"
var httpdContainerName = "httpd"
var httpdBinary = "/usr/local/apache2/bin/httpd"
var proxyBalancerConf = "/usr/local/apache2/conf/balancer/proxy_balancer.conf"
var tomcatImage = "tomcat:.*"
var tomcatLabels = map[string]string{ /* This should be kept here even when empty to ensure the script runs properly */
	"category": "test-dynamic-reverse-proxy",
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
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

func ipAlreadyExists(podIP string, containerPort string) bool {
	fmt.Println(podIP)
	fmt.Println(containerPort)
	//TODO Check how exec.Command behaves when grep returns 1
	// grep_regex = pod_ip+":"+container_port
	// ip_already_exists_command = [
	//     "kubectl", "exec", httpd_pod_name, "--", "grep", grep_regex, proxy_balancer_conf]
	// try:
	//     subprocess.check_call(ip_already_exists_command, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT)
	//     return True
	// except subprocess.CalledProcessError as e:
	//     ### grep returns 1 when no matches are found ###
	//     if e.returncode == 1:
	//         return False
	//     else:
	// 		raise
	return false
}

func addToBalancer(podIP string, containerPort string) {
	sedExpression := "s|\\(<Proxy \"balancer:.*>\\)|\\1\\n    BalancerMember \"http://" + podIP + ":" + containerPort + "\"|"
	exec.Command("kubectl", "exec", httpdPodName, "--", "sed", "-i", sedExpression, proxyBalancerConf)
}

func removeFromBalancer(podIP string, containerPort string) {
	sedExpression := "/    BalancerMember \"http:\\/\\/" + podIP + ":" + containerPort + "\"/d"
	exec.Command("kubectl", "exec", httpdPodName, "--", "sed", "-i", sedExpression, proxyBalancerConf)
}

func reloadHttpd() {
	fmt.Println("Reloading httpd configuration.")
	exec.Command("kubectl", "exec", "httpd", "--container", httpdContainerName, "--", httpdBinary, "-k", "graceful")
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func podContainsHttpd(pod *v1.Pod) bool {
	return (pod.ObjectMeta.Name == httpdPodName)
}

func getHttpdContainerStatus(pod *v1.Pod) v1.ContainerStatus {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == httpdContainerName {
			return containerStatus
		}
	}
	return v1.ContainerStatus{}
}

func getHttpdContainerSpecs(pod *v1.Pod) v1.Container {
	for _, containerSpecs := range pod.Spec.Containers {
		if containerSpecs.Name == httpdContainerName {
			return containerSpecs
		}
	}
	return v1.Container{}
}

func handleHttpdPod(pod *v1.Pod, tomcats map[string]string, httpdOnline bool) bool {
	podIP := pod.Status.PodIP
	podName := pod.ObjectMeta.Name
	// Empty line to separate log events
	fmt.Println("")
	fmt.Printf("Namespace: %s Pod Name: %s\n", pod.ObjectMeta.Namespace, podName)

	if podIP != "" {

		if len(pod.Status.Conditions) > 0 && len(pod.Status.ContainerStatuses) > 0 {
			httpdStatus := getHttpdContainerStatus(pod)
			// httpdSpecs := getHttpdContainerSpecs(pod)
			containerActive := containerIsActive(pod, httpdStatus)

			if containerActive {

				if !httpdOnline {
					fmt.Println("httpd is online")
				} else {
					fmt.Println("httpd modified")
				}
				fmt.Println("Checking for missing balancer members")
				reloadNeeded := false
				for tomcatIP, tomcatPort := range tomcats {
					if ipAlreadyExists(tomcatIP, tomcatPort) {

						fmt.Println("Skipping " + tomcatIP + ":" + tomcatPort)
						fmt.Println("Reason: Already exists in " + proxyBalancerConf)

					} else {

						fmt.Println("Adding " + tomcatIP + ":" + tomcatPort + " to " + proxyBalancerConf)
						addToBalancer(tomcatIP, tomcatPort)
						reloadNeeded = true
					}
				}
				if reloadNeeded {
					fmt.Println("Added all missing balancer members")
					reloadHttpd()
				} else {
					fmt.Println("No missing balancer members found")
				}

				httpdOnline = true

			} else if pod.ObjectMeta.DeletionGracePeriodSeconds != nil ||
				pod.ObjectMeta.DeletionTimestamp != nil {

				print("httpd is being deleted")
				httpdOnline = false

			} else {
				fmt.Println("httpd on pod \"" + podName + "\" is offline.")
				httpdOnline = false
			}
		} else {
			fmt.Println("Pod status elements for pod \"" + podName + "\" are not initialized yet.")
		}
	} else {
		fmt.Println("Pod \"" + podName + "\" has no IP yet.")
	}
	return httpdOnline
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// def pod_contains_tomcat(pod):
//     images_match = False

//     for container in pod.spec.containers:
//         if re.match(tomcat_image, container.image) is not None:
//             images_match = True
//             break
//     if images_match and \
//         tomcat_labels is not None and \
//         tomcat_labels and \
//         pod.metadata.labels is not None and \
//         tomcat_labels.items() <= pod.metadata.labels.items():

//         return True

//     return False

// def get_tomcat_container_status(pod):
//     for container_status in pod.status.container_statuses:
//         if re.match(tomcat_image, container_status.image):
//             return container_status
//         else:
//             return None

// def get_tomcat_container_specs(containers_specs, tomcat_status):
//     for container_specs in containers_specs:
//         if container_specs.image == tomcat_status.image:
//             return container_specs
//         else:
//             return None

// def tomcat_responds(pod_name, pod_ip, tomcat_port):
//     i = 0
//     while ++i <= 10:
//         ret = requests.get("http://"+ pod_ip +":"+ tomcat_port)
//         if ret.status_code == 200:
//             return True
//         time.sleep(1)
//         if i == 10:
//             return False

// def handle_tomcat_pod(pod,tomcats,httpd_online):
//     pod_ip = get_pod_IP(pod)
//     pod_name = pod.metadata.name
//     ### Empty line to separate log events ###
//     print()
//     print("Namespace: %s Pod Name: %s" %
//                 (pod.metadata.namespace, pod.metadata.name))
//     if pod_ip is not None:
//         if pod.status.conditions is not None and \
//             pod.status.container_statuses is not None:

//             tomcat_status = get_tomcat_container_status(pod)
//             tomcat_specs = get_tomcat_container_specs(pod.spec.containers, tomcat_status)
//             container_active = container_is_active(pod, tomcat_status)
//             ## We assume that only one port is exposed on the tomcat container ###
//             container_port = str(tomcat_specs.ports[0].container_port)

//             if pod_ip in tomcats and not container_active:
//                 if httpd_online:
//                     print("Tomcat server in pod \""+pod_name +"\" went offline/crashed, removing it from proxy_balancer.conf")
//                     remove_from_balancer(pod_ip, container_port)
//                     reload_httpd()
//                 tomcats.pop(pod_ip)

//             else if pod_ip not in tomcats and container_active:

//                 if tomcat_responds(pod_name, pod_ip, container_port):
//                     if httpd_online:
//                         if ip_already_exists(pod_ip, container_port):
//                             print("IP of pod \"" + pod_name + "\" already in proxy_balancer.conf. Skipping...")
//                         else:
//                             print("Tomcat server in pod \""+pod_name +"\" is online, adding it to proxy_balancer.conf")
//                             add_to_balancer(pod_ip, container_port)
//                             reload_httpd()
//                     else:
//                         print("Tomcat server in pod \""+pod_name +"\" is online, but httpd isn't")
//                         print("The tomcat server will be added to httpd's balancer members when it's back online")
//                     tomcats[pod_ip] = container_port

//                 else:
//                     print("Can't get a response from tomcat, check pod "+pod_name)

//             ### Checks if tomcat is in the process of being deleted to prevent unnecessary error messages ###
//             else if pod_ip not in tomcats and \
//                 (pod.metadata.deletion_grace_period_seconds is not None or
//                 pod.metadata.deletion_timestamp is not None):

//                 print("Pod \""+pod_name+"\" is being deleted.")

//             else:
//                 print("Tomcat on pod \""+pod_name+"\" is offline.")
//         else:
//             print("Pod status elements for pod \"" +pod_name+"\" are not initialized yet.")
//     else:
//         print("Pod \""+pod_name+"\" has no IP yet.")

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func main() {
	var kubeconfig *string
	tomcats := make(map[string]string)
	httpdOnline := false
	if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	api := clientset.CoreV1()
	listOptions := metav1.ListOptions{
		// LabelSelector: label,
		// FieldSelector: field,
	}

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
		// if podContainsTomcat(pod){
		// 	handleTomcatPod(pod,tomcats,httpdOnline)
		// }

	}
}
